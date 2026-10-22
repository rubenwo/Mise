package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"sync/atomic"
	"time"
)

// ProviderType identifies which LLM API a provider speaks.
type ProviderType string

const (
	ProviderTypeOllama       ProviderType = "ollama"
	ProviderTypeOpenAICompat ProviderType = "openai_compat"
)

type Client struct {
	baseURL      string
	model        string
	providerType ProviderType
	httpClient   *http.Client
	healthy      atomic.Bool
	providerID   int
	tags         []string
	// numCtx overrides Ollama's context window when > 0. Ollama otherwise
	// picks a VRAM-based default that can be as small as 4k tokens, which
	// silently truncates long tool loops.
	numCtx int
}

func NewClient(cfg ProviderConfig) *Client {
	c := &Client{
		baseURL:      cfg.Host,
		model:        cfg.Model,
		providerType: cfg.ProviderType,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		providerID: cfg.ProviderID,
		tags:       cfg.Tags,
		numCtx:     cfg.NumCtx,
	}
	c.healthy.Store(true)
	return c
}

func (c *Client) hasTag(tag string) bool {
	return slices.Contains(c.tags, tag)
}

// Message is the shared message type for both Ollama and OpenAI-compat APIs.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Images     []string   `json:"images,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // OpenAI-compat: correlates tool result to prior tool call
}

// ToolCall is the shared tool call type. ID is populated for OpenAI-compat responses.
type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ChatResponse is the normalized response returned regardless of underlying provider.
type ChatResponse struct {
	Message       Message `json:"message"`
	Done          bool    `json:"done"`
	DoneReason    string  `json:"done_reason,omitempty"`
	TotalDuration int64   `json:"total_duration,omitempty"`
}

// ---- Ollama-native wire types ----

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []Message       `json:"messages"`
	Tools    []Tool          `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
	Format   json.RawMessage `json:"format,omitempty"` // "json" or a JSON schema
	Options  *ollamaOptions  `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature *float64 `json:"temperature,omitempty"`
	NumCtx      int      `json:"num_ctx,omitempty"`
}

type ollamaChatResponse struct {
	Message       Message `json:"message"`
	Done          bool    `json:"done"`
	DoneReason    string  `json:"done_reason,omitempty"`
	TotalDuration int64   `json:"total_duration,omitempty"`
}

// ---- OpenAI-compat wire types ----

type openAIChatRequest struct {
	Model          string            `json:"model"`
	Messages       []openAIMessage   `json:"messages"`
	Tools          []Tool            `json:"tools,omitempty"`
	Stream         bool              `json:"stream"`
	ResponseFormat *openAIRespFormat `json:"response_format,omitempty"`
	Temperature    *float64          `json:"temperature,omitempty"`
}

type openAIRespFormat struct {
	Type       string            `json:"type"`
	JSONSchema *openAIJSONSchema `json:"json_schema,omitempty"`
}

type openAIJSONSchema struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
}

// openAIMessage mirrors the OpenAI wire format. Content is a pointer so it can
// be null for assistant messages that only contain tool calls.
type openAIMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function openAIToolCallFunction `json:"function"`
}

// openAIToolCallFunction uses a plain string for Arguments (JSON-encoded string,
// not a raw JSON object like Ollama uses).
type openAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
}

// ---- Public API ----

func (c *Client) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	if c.providerType == ProviderTypeOpenAICompat {
		return c.doChatOpenAI(ctx, messages, tools, false)
	}
	return c.doChatOllama(ctx, ollamaChatRequest{
		Model:    c.model,
		Messages: messages,
		Tools:    tools,
		Stream:   false,
		Options:  c.ollamaOptions(nil),
	})
}

// ollamaOptions returns request options, or nil when there is nothing to set.
func (c *Client) ollamaOptions(temperature *float64) *ollamaOptions {
	if temperature == nil && c.numCtx <= 0 {
		return nil
	}
	return &ollamaOptions{Temperature: temperature, NumCtx: c.numCtx}
}

// jsonModeTemperature is used for all JSON-strict calls. Lower temperature
// reduces schema drift on small models without harming recipe creativity,
// which is shaped by the prompt rather than randomness.
var jsonModeTemperature = 0.2

// ChatJSON calls the model requesting a JSON response (no tools), at a low
// temperature so outputs stay on-format.
//
// With a non-nil schema the output is grammar-constrained to that JSON schema
// (Ollama structured outputs / OpenAI json_schema), so required fields and
// types are guaranteed rather than merely requested. A nil schema falls back
// to plain JSON mode.
func (c *Client) ChatJSON(ctx context.Context, messages []Message, schema *Schema) (*ChatResponse, error) {
	if c.providerType == ProviderTypeOpenAICompat {
		format := &openAIRespFormat{Type: "json_object"}
		if schema != nil {
			format = &openAIRespFormat{Type: "json_schema", JSONSchema: &openAIJSONSchema{Name: schema.Name, Schema: schema.JSON}}
		}
		return c.doChatOpenAIRaw(ctx, openAIChatRequest{
			Model:          c.model,
			Messages:       toOpenAIMessages(messages),
			Stream:         false,
			ResponseFormat: format,
			Temperature:    &jsonModeTemperature,
		})
	}
	format := json.RawMessage(`"json"`)
	if schema != nil {
		format = schema.JSON
	}
	return c.doChatOllama(ctx, ollamaChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Format:   format,
		Options:  c.ollamaOptions(&jsonModeTemperature),
	})
}

// ---- Ollama implementation ----

func (c *Client) doChatOllama(ctx context.Context, req ollamaChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling ollama: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(respBody))
	}

	var r ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if r.DoneReason == "length" {
		log.Printf("Warning: %s hit its token limit and the output is truncated (consider raising ollama.num_ctx)", c.model)
	}

	return &ChatResponse{
		Message:       r.Message,
		Done:          r.Done,
		DoneReason:    r.DoneReason,
		TotalDuration: r.TotalDuration,
	}, nil
}

// ---- OpenAI-compat implementation ----

func (c *Client) doChatOpenAI(ctx context.Context, messages []Message, tools []Tool, jsonMode bool) (*ChatResponse, error) {
	req := openAIChatRequest{
		Model:    c.model,
		Messages: toOpenAIMessages(messages),
		Tools:    tools,
		Stream:   false,
	}
	if jsonMode {
		req.ResponseFormat = &openAIRespFormat{Type: "json_object"}
	}
	return c.doChatOpenAIRaw(ctx, req)
}

func (c *Client) doChatOpenAIRaw(ctx context.Context, req openAIChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling openai-compat provider: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned %d: %s", resp.StatusCode, string(respBody))
	}

	var r openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if len(r.Choices) == 0 {
		return nil, fmt.Errorf("provider returned no choices")
	}

	choice := r.Choices[0]
	msg := fromOpenAIMessage(choice.Message)

	return &ChatResponse{
		Message:    msg,
		Done:       choice.FinishReason != "",
		DoneReason: choice.FinishReason,
	}, nil
}

// toOpenAIMessages converts our internal Message slice to the OpenAI wire format.
func toOpenAIMessages(messages []Message) []openAIMessage {
	out := make([]openAIMessage, len(messages))
	for i, m := range messages {
		om := openAIMessage{
			Role:       m.Role,
			ToolCallID: m.ToolCallID,
		}
		// Assistant messages with only tool calls may have null content in OpenAI format.
		if len(m.ToolCalls) > 0 && m.Content == "" {
			om.Content = nil
		} else {
			s := m.Content
			om.Content = &s
		}
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, openAIToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: openAIToolCallFunction{
					Name:      tc.Function.Name,
					Arguments: string(tc.Function.Arguments),
				},
			})
		}
		out[i] = om
	}
	return out
}

// fromOpenAIMessage converts an OpenAI response message to our internal Message type.
func fromOpenAIMessage(om openAIMessage) Message {
	content := ""
	if om.Content != nil {
		content = *om.Content
	}
	msg := Message{
		Role:       om.Role,
		Content:    content,
		ToolCallID: om.ToolCallID,
	}
	for _, tc := range om.ToolCalls {
		// OpenAI arguments is a JSON string; wrap it directly as json.RawMessage.
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID: tc.ID,
			Function: ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			},
		})
	}
	return msg
}

// ---- Model management (Ollama-only) ----

func (c *Client) EnsureModel(ctx context.Context) error {
	if c.providerType != ProviderTypeOllama {
		return nil // non-Ollama providers manage their own models
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("checking models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return fmt.Errorf("decoding tags: %w", err)
	}

	for _, m := range tags.Models {
		if m.Name == c.model || m.Name == c.model+":latest" {
			return nil
		}
	}

	log.Printf("Model %s not found, pulling...", c.model)
	return c.pullModel(ctx)
}

func (c *Client) pullModel(ctx context.Context) error {
	body, _ := json.Marshal(map[string]any{
		"name":   c.model,
		"stream": false,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("pulling model: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pull failed with %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("Model %s pulled successfully", c.model)
	return nil
}

func (c *Client) Model() string {
	return c.model
}

func (c *Client) IsHealthy(ctx context.Context) bool {
	var endpoint string
	switch c.providerType {
	case ProviderTypeOpenAICompat:
		endpoint = c.baseURL + "/v1/models"
	default:
		endpoint = c.baseURL + "/api/tags"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}
