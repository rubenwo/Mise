package llm

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/rubenwo/mise/internal/database"
)

type ProviderConfig struct {
	Host         string
	Model        string
	ProviderType ProviderType
	Timeout      time.Duration
	ProviderID   int
	Tags         []string
	NumCtx       int  // Ollama num_ctx override; 0 = server default
	Think        bool // enable reasoning for tool-loop/chat calls
}

type ClientPool struct {
	mu      sync.Mutex
	clients []*Client
	next    int
	// residencies outlive Reload so the Warmer keeps track of models Mise
	// loaded before a settings change.
	residencies map[string]*residency
}

func NewClientPool(providers []ProviderConfig) *ClientPool {
	p := &ClientPool{residencies: map[string]*residency{}}
	p.buildClients(providers)
	return p
}

func (p *ClientPool) buildClients(providers []ProviderConfig) {
	clients := make([]*Client, 0, len(providers))
	for _, prov := range providers {
		c := NewClient(prov)
		key := residencyKey(prov.Host, prov.Model)
		if r, ok := p.residencies[key]; ok {
			c.res = r
		} else {
			p.residencies[key] = c.res
		}
		clients = append(clients, c)
	}
	p.clients = clients
	p.next = 0
}

func (p *ClientPool) Reload(providers []ProviderConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buildClients(providers)
}

// FeatureStatus returns the availability of a feature tag:
//
//	"available"    — at least one healthy provider has this tag
//	"offline"      — a provider has the tag but none are currently healthy
//	"unconfigured" — no provider has this tag at all
func (p *ClientPool) FeatureStatus(tag string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	configured := false
	for _, c := range p.clients {
		if c.hasTag(tag) {
			configured = true
			if c.healthy.Load() {
				return "available"
			}
		}
	}
	if configured {
		return "offline"
	}
	return "unconfigured"
}

// AcquireWithTag returns a healthy client that has the given tag, using
// round-robin across matching clients. Returns nil if no healthy tagged
// client exists — callers must handle nil and return an appropriate error.
// There is no fallback to untagged providers.
func (p *ClientPool) AcquireWithTag(tag string) *Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	startIdx := p.next
	for i := 0; i < len(p.clients); i++ {
		idx := (startIdx + i) % len(p.clients)
		c := p.clients[idx]
		if c.healthy.Load() && c.hasTag(tag) {
			p.next = idx + 1
			return c
		}
	}
	return nil
}

// clientForTag returns the first healthy client with tag without advancing
// the round-robin, for looking at a feature's model rather than calling it.
func (p *ClientPool) clientForTag(tag string) *Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.clients {
		if c.healthy.Load() && c.hasTag(tag) {
			return c
		}
	}
	return nil
}

func (p *ClientPool) Model() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.clients) == 0 {
		return ""
	}
	if len(p.clients) == 1 {
		return p.clients[0].Model()
	}
	models := make([]string, len(p.clients))
	for i, c := range p.clients {
		models[i] = c.Model()
	}
	return strings.Join(models, ", ")
}

func (p *ClientPool) Clients() []*Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*Client, len(p.clients))
	copy(out, p.clients)
	return out
}

// healthCheckTimeout bounds a single provider probe so one unreachable host
// cannot stall the checks for the others.
const healthCheckTimeout = 5 * time.Second

// StartHealthChecker probes every provider immediately and then on each
// interval until ctx is cancelled.
func (p *ClientPool) StartHealthChecker(ctx context.Context, interval time.Duration, db *database.Queries) {
	go func() {
		p.checkHealth(ctx, db)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.checkHealth(ctx, db)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (p *ClientPool) checkHealth(ctx context.Context, db *database.Queries) {
	var wg sync.WaitGroup
	for _, c := range p.Clients() {
		wg.Go(func() {
			probeCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
			healthy := c.IsHealthy(probeCtx)
			cancel()
			c.healthy.Store(healthy)

			status := "healthy"
			var lastError *string
			if !healthy {
				status = "unhealthy"
				msg := "health check failed"
				lastError = &msg
			}
			_ = db.UpdateProviderHealthStatus(ctx, c.providerID, status, lastError)
		})
	}
	wg.Wait()
}
