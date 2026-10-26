package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// keepAlive is how long Ollama keeps a model loaded after Mise's last
// request to it. Ollama's default of 5 minutes holds the V100s at high clocks
// for long after a question; the Warmer extends it while the user is around
// and unloads early when they leave.
const keepAlive = 3 * time.Minute

// residency is what Mise knows about one model on one Ollama host, shared by
// every client (provider) that uses it.
type residency struct {
	inflight atomic.Int32
	mu       sync.Mutex
	// expected is when Ollama should unload the model if nobody but Mise has
	// used it since Mise's last request; zero if Mise never used it.
	expected time.Time
}

func (r *residency) begin() { r.inflight.Add(1) }

func (r *residency) end(keepAliveFor time.Duration) {
	r.mu.Lock()
	r.expected = time.Now().Add(keepAliveFor)
	r.mu.Unlock()
	r.inflight.Add(-1)
}

func (r *residency) expectedExpiry() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.expected
}

func residencyKey(host, model string) string {
	return strings.TrimRight(host, "/") + "|" + canonicalModel(model)
}

// canonicalModel adds the implicit ":latest" tag, as Ollama reports names.
func canonicalModel(name string) string {
	if strings.Contains(name, ":") {
		return name
	}
	return name + ":latest"
}

// loadedModel is one entry of Ollama's /api/ps.
type loadedModel struct {
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
}

// loadedModels lists the models loaded on the client's Ollama host.
func (c *Client) loadedModels(ctx context.Context) ([]loadedModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/ps", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama /api/ps returned %d", resp.StatusCode)
	}
	var out struct {
		Models []loadedModel `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Models, nil
}

// findLoaded returns the client's model among loaded, if present.
func (c *Client) findLoaded(loaded []loadedModel) (loadedModel, bool) {
	want := canonicalModel(c.model)
	for _, m := range loaded {
		if canonicalModel(m.Name) == want {
			return m, true
		}
	}
	return loadedModel{}, false
}

// load loads the model, or extends how long it stays loaded, without
// generating anything. It sends the same options as real requests: a
// different num_ctx would make Ollama reload the model on the first real
// call, wasting the warm-up.
func (c *Client) load(ctx context.Context) error {
	c.res.begin()
	defer c.res.end(keepAlive)
	return c.generateNoPrompt(ctx, map[string]any{
		"model":      c.model,
		"keep_alive": int(keepAlive.Seconds()),
		"options":    c.ollamaOptions(nil),
	})
}

// unload asks Ollama to free the model now.
func (c *Client) unload(ctx context.Context) error {
	return c.generateNoPrompt(ctx, map[string]any{"model": c.model, "keep_alive": 0})
}

func (c *Client) generateNoPrompt(ctx context.Context, body map[string]any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama returned %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
