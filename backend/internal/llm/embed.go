package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Embed returns one embedding vector per input, in input order. Providers
// tagged "embedding" serve it (e.g. embeddinggemma on Ollama).
func (c *Client) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if c.providerType == ProviderTypeOpenAICompat {
		var r struct {
			Data []struct {
				Index     int       `json:"index"`
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}
		if err := c.postJSON(ctx, "/v1/embeddings", map[string]any{"model": c.model, "input": inputs}, &r); err != nil {
			return nil, err
		}
		out := make([][]float32, len(inputs))
		for _, d := range r.Data {
			if d.Index >= 0 && d.Index < len(out) {
				out[d.Index] = d.Embedding
			}
		}
		return checkEmbeddings(out)
	}
	var r struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := c.postJSON(ctx, "/api/embed", map[string]any{"model": c.model, "input": inputs}, &r); err != nil {
		return nil, err
	}
	if len(r.Embeddings) != len(inputs) {
		return nil, fmt.Errorf("embedding: got %d vectors for %d inputs", len(r.Embeddings), len(inputs))
	}
	return checkEmbeddings(r.Embeddings)
}

func checkEmbeddings(vs [][]float32) ([][]float32, error) {
	for i, v := range vs {
		if len(v) == 0 {
			return nil, fmt.Errorf("embedding: input %d has no vector", i)
		}
	}
	return vs, nil
}

func (c *Client) postJSON(ctx context.Context, path string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling %s: %w", c.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s returned %d: %s", path, resp.StatusCode, string(b))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}
