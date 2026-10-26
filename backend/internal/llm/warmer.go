package llm

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"
)

// InteractiveTags are the features the user waits on. Their model is worth
// loading ahead of time; background-generation is not.
var InteractiveTags = []string{"chat", "generation", "search", "translation", "deduplication", "inventory"}

// Warmer loads the interactive model before the user needs it and unloads
// what Mise loaded once they leave, so answers start quickly without the
// V100s idling at high clocks.
//
// The frontend reports presence: Active with the features likely to be used
// right away (on a recipe page, the cooking chat), Away when the tab is
// hidden. A model is only loaded if doing so evicts nothing: when the host
// already has another model loaded (a background batch on the 31b, an
// experiment, an agent session), the warm-up is skipped and only a real
// request may take its place. Unloading likewise only touches a model that
// Mise used last and that is idle.
type Warmer struct {
	pool *ClientPool
	now  func() time.Time

	// awayGrace is how long a hidden tab may stay hidden, and idleAfter how
	// long Mise may go without any presence signal, before models are freed.
	awayGrace, idleAfter time.Duration
	// recheck limits how often one model is checked for a warm-up.
	recheck time.Duration

	mu        sync.Mutex
	lastSeen  time.Time
	awaySince time.Time // zero while present
	released  bool      // models already freed for the current absence
	checked   map[*residency]time.Time
	busy      map[*residency]bool
}

func NewWarmer(pool *ClientPool) *Warmer {
	return &Warmer{
		pool:      pool,
		now:       time.Now,
		awayGrace: time.Minute,
		idleAfter: 3 * time.Minute,
		recheck:   15 * time.Second,
		checked:   map[*residency]time.Time{},
		busy:      map[*residency]bool{},
	}
}

// Active records that the user is using Mise. tags name the features likely
// to be used right away; their models are loaded, or kept loaded, unless that
// would evict another model.
func (w *Warmer) Active(tags []string) {
	w.mu.Lock()
	now := w.now()
	w.lastSeen, w.awaySince, w.released = now, time.Time{}, false
	w.mu.Unlock()

	seen := map[*residency]bool{}
	for _, tag := range tags {
		if !slices.Contains(InteractiveTags, tag) {
			continue
		}
		c := w.pool.clientForTag(tag)
		if c == nil || c.providerType != ProviderTypeOllama || seen[c.res] {
			continue
		}
		seen[c.res] = true
		if w.claim(c.res, true) {
			go func() {
				defer w.done(c.res)
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				w.warm(ctx, c)
			}()
		}
	}
}

// Away records that the user left (the tab was hidden). Models are freed
// after awayGrace unless the user comes back.
func (w *Warmer) Away() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.awaySince.IsZero() {
		w.awaySince = w.now()
	}
}

// BackgroundDone frees the model a background job (tag) used, unless the
// user is around and the same model also serves features they may use.
func (w *Warmer) BackgroundDone(tag string) {
	c := w.pool.clientForTag(tag)
	if c == nil || c.providerType != ProviderTypeOllama {
		return
	}
	if w.present() && slices.ContainsFunc(InteractiveTags, func(t string) bool {
		ic := w.pool.clientForTag(t)
		return ic != nil && ic.res == c.res
	}) {
		return
	}
	if w.claim(c.res, false) {
		defer w.done(c.res)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		w.release(ctx, c)
	}
}

// Start frees Mise's models once the user has been away or idle long enough.
func (w *Warmer) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.releaseIfGone(ctx)
			}
		}
	}()
}

func (w *Warmer) present() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.presentLocked()
}

// presentLocked reports whether the user is around: a recent presence
// signal and no tab hidden for longer than the grace period.
func (w *Warmer) presentLocked() bool {
	now := w.now()
	return !w.lastSeen.IsZero() && now.Sub(w.lastSeen) < w.idleAfter &&
		(w.awaySince.IsZero() || now.Sub(w.awaySince) < w.awayGrace)
}

func (w *Warmer) releaseIfGone(ctx context.Context) {
	w.mu.Lock()
	gone := !w.lastSeen.IsZero() && !w.released && !w.presentLocked()
	if gone {
		w.released = true
	}
	w.mu.Unlock()
	if !gone {
		return
	}
	seen := map[*residency]bool{}
	for _, c := range w.pool.Clients() {
		if c.providerType != ProviderTypeOllama || seen[c.res] {
			continue
		}
		seen[c.res] = true
		if w.claim(c.res, false) {
			w.release(ctx, c)
			w.done(c.res)
		}
	}
}

// claim marks res busy so warm-ups and releases of one model never overlap;
// a warm-up claim also respects the recheck interval.
func (w *Warmer) claim(res *residency, warmup bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.busy[res] {
		return false
	}
	if warmup {
		now := w.now()
		if now.Sub(w.checked[res]) < w.recheck {
			return false
		}
		w.checked[res] = now
	}
	w.busy[res] = true
	return true
}

func (w *Warmer) done(res *residency) {
	w.mu.Lock()
	delete(w.busy, res)
	w.mu.Unlock()
}

// warm loads c's model if nothing else is loaded on its host, or extends it
// if it is loaded and due to expire soon.
func (w *Warmer) warm(ctx context.Context, c *Client) {
	loaded, err := c.loadedModels(ctx)
	if err != nil {
		slog.Debug("warm-up: cannot list loaded models", "model", c.model, "err", err)
		return
	}
	if m, ok := c.findLoaded(loaded); ok {
		// Already loaded. Extend only if it would expire soon: refreshing
		// with Mise's keep-alive could shorten a longer one another client set.
		if c.res.inflight.Load() > 0 || m.ExpiresAt.After(w.now().Add(keepAlive/2)) {
			return
		}
		if err := c.load(ctx); err != nil {
			slog.Warn("warm-up: extending model failed", "model", c.model, "err", err)
		}
		return
	}
	if len(loaded) > 0 {
		slog.Info("warm-up skipped: another model is loaded", "model", c.model, "loaded", loaded[0].Name)
		return
	}
	start := w.now()
	if err := c.load(ctx); err != nil {
		slog.Warn("warm-up failed", "model", c.model, "err", err)
		return
	}
	slog.Info("warmed up model", "model", c.model, "took", w.now().Sub(start).Round(time.Millisecond))
}

// release unloads c's model if Mise used it last and nothing is running on it.
func (w *Warmer) release(ctx context.Context, c *Client) {
	expected := c.res.expectedExpiry()
	if expected.IsZero() || c.res.inflight.Load() > 0 {
		return
	}
	loaded, err := c.loadedModels(ctx)
	if err != nil {
		return
	}
	m, ok := c.findLoaded(loaded)
	// A later expiry than Mise's own means another client used the model
	// since (Ollama resets the expiry on every request).
	if !ok || m.ExpiresAt.After(expected.Add(15*time.Second)) {
		return
	}
	if err := c.unload(ctx); err != nil {
		slog.Warn("unloading model failed", "model", c.model, "err", err)
		return
	}
	slog.Info("unloaded idle model", "model", c.model)
}
