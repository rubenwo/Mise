package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

type Client struct {
	baseURL      string
	model        string
	httpClient   *http.Client
	healthy      atomic.Bool
	lastCheck    time.Time
	providerID   int
	tags         []string
}

func NewClient(baseURL, model string, timeout time.Duration, providerID int, tags []string) *Client {
	c := &Client{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		providerID: providerID,
		tags:       tags,
	}
	c.healthy.Store(true)
	c.lastCheck = time.Now()
	return c
}

func (c *Client) hasTag(tag string) bool {
	for _, t := range c.tags {
		if t == tag {
			return true
		}
	}
	return false
}

type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream"`
	Format   string    `json:"format,omitempty"`
}

type ChatResponse struct {
	Message      Message `json:"message"`
	Done         bool    `json:"done"`
	DoneReason   string  `json:"done_reason,omitempty"`
	TotalDuration int64  `json:"total_duration,omitempty"`
}

func (c *Client) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	return c.doChat(ctx, ChatRequest{
		Model:    c.model,
		Messages: messages,
		Tools:    tools,
		Stream:   false,
	})
}

// ChatJSON calls the model requesting a JSON-formatted response (no tools, format:"json").
func (c *Client) ChatJSON(ctx context.Context, messages []Message) (*ChatResponse, error) {
	return c.doChat(ctx, ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Format:   "json",
	})
}

func (c *Client) doChat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
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

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &chatResp, nil
}

func (c *Client) EnsureModel(ctx context.Context) error {
	resp, err := http.Get(c.baseURL + "/api/tags")
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
	resp, err := http.Get(c.baseURL + "/api/tags")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}
