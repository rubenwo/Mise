package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeOllama keeps a set of loaded models with expiry times and records the
// requests that load or unload them.
type fakeOllama struct {
	mu     sync.Mutex
	loaded map[string]time.Time
	calls  []fakeCall
}

type fakeCall struct {
	path      string
	model     string
	keepAlive int
	numCtx    int
}

func newFakeOllama(t *testing.T) (*fakeOllama, *httptest.Server) {
	f := &fakeOllama{loaded: map[string]time.Time{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path == "/api/ps" {
			var models []loadedModel
			for name, exp := range f.loaded {
				models = append(models, loadedModel{Name: name, ExpiresAt: exp})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"models": models})
			return
		}
		var req struct {
			Model     string `json:"model"`
			KeepAlive *int   `json:"keep_alive"`
			Options   struct {
				NumCtx int `json:"num_ctx"`
			} `json:"options"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		ka := 300 // Ollama's default
		if req.KeepAlive != nil {
			ka = *req.KeepAlive
		}
		f.calls = append(f.calls, fakeCall{r.URL.Path, req.Model, ka, req.Options.NumCtx})
		if ka == 0 {
			delete(f.loaded, req.Model)
		} else {
			f.loaded[req.Model] = time.Now().Add(time.Duration(ka) * time.Second)
		}
		if r.URL.Path == "/api/chat" {
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"role": "assistant", "content": "ok"}, "done": true})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"done": true})
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeOllama) generateCalls() []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []fakeCall
	for _, c := range f.calls {
		if c.path == "/api/generate" {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeOllama) isLoaded(model string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.loaded[model]
	return ok
}

func (f *fakeOllama) setLoaded(model string, expires time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loaded[model] = expires
}

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newTestWarmer(url string) (*Warmer, *ClientPool, *testClock) {
	pool := NewClientPool([]ProviderConfig{
		{Host: url, Model: "gemma4:26b", ProviderType: ProviderTypeOllama, Timeout: 5 * time.Second, NumCtx: 16384,
			Tags: []string{"chat", "generation", "search", "translation"}},
		{Host: url, Model: "gemma4:31b", ProviderType: ProviderTypeOllama, Timeout: 5 * time.Second, NumCtx: 16384,
			Tags: []string{"background-generation"}},
	})
	clock := &testClock{t: time.Now()}
	w := NewWarmer(pool)
	w.now = clock.now
	return w, pool, clock
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWarmLoadsWithRealRequestOptionsWhenGPUIsFree(t *testing.T) {
	f, srv := newFakeOllama(t)
	w, _, _ := newTestWarmer(srv.URL)
	w.Active([]string{"chat"})
	eventually(t, "the chat model to load", func() bool { return f.isLoaded("gemma4:26b") })
	calls := f.generateCalls()
	if len(calls) != 1 || calls[0].numCtx != 16384 || calls[0].keepAlive != int(keepAlive.Seconds()) {
		t.Errorf("warm-up calls = %+v, want one load with num_ctx 16384 and Mise's keep-alive", calls)
	}
}

func TestWarmNeverEvictsAnotherModel(t *testing.T) {
	f, srv := newFakeOllama(t)
	f.setLoaded("gemma4:31b", time.Now().Add(time.Minute))
	w, _, _ := newTestWarmer(srv.URL)
	w.Active([]string{"chat"})
	eventually(t, "the warm-up check to finish", func() bool { return !w.busyWith() })
	if len(f.generateCalls()) != 0 || !f.isLoaded("gemma4:31b") {
		t.Errorf("warm-up touched the GPU while another model was loaded: %+v", f.generateCalls())
	}
}

func TestWarmDoesNotShortenAnotherClientsKeepAlive(t *testing.T) {
	f, srv := newFakeOllama(t)
	f.setLoaded("gemma4:26b", time.Now().Add(time.Hour)) // e.g. an agent session
	w, _, _ := newTestWarmer(srv.URL)
	w.Active([]string{"chat"})
	eventually(t, "the warm-up check to finish", func() bool { return !w.busyWith() })
	if calls := f.generateCalls(); len(calls) != 0 {
		t.Errorf("refreshed a model someone else holds longer: %+v", calls)
	}
}

func TestChatRequestsSetMisesKeepAlive(t *testing.T) {
	f, srv := newFakeOllama(t)
	_, pool, _ := newTestWarmer(srv.URL)
	if _, err := pool.AcquireWithTag("chat").Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := f.calls[0].keepAlive; got != int(keepAlive.Seconds()) {
		t.Errorf("chat keep_alive = %d, want %d", got, int(keepAlive.Seconds()))
	}
}

func TestLeavingUnloadsOnlyWhatMiseUsedLast(t *testing.T) {
	f, srv := newFakeOllama(t)
	w, pool, clock := newTestWarmer(srv.URL)
	if _, err := pool.AcquireWithTag("chat").Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatal(err)
	}
	w.Active(nil)
	w.Away()
	clock.advance(30 * time.Second)
	w.releaseIfGone(context.Background())
	if !f.isLoaded("gemma4:26b") {
		t.Fatal("unloaded within the away grace period")
	}
	clock.advance(time.Minute)
	w.releaseIfGone(context.Background())
	if f.isLoaded("gemma4:26b") {
		t.Error("model Mise used last stayed loaded after the user left")
	}

	// Another client used the model after Mise: leave it alone.
	if _, err := pool.AcquireWithTag("chat").Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatal(err)
	}
	f.setLoaded("gemma4:26b", time.Now().Add(10*time.Minute))
	w.Active(nil)
	w.Away()
	clock.advance(2 * time.Minute)
	w.releaseIfGone(context.Background())
	if !f.isLoaded("gemma4:26b") {
		t.Error("unloaded a model another client is using")
	}
}

func TestIdleUserCountsAsGone(t *testing.T) {
	f, srv := newFakeOllama(t)
	w, pool, clock := newTestWarmer(srv.URL)
	if _, err := pool.AcquireWithTag("chat").Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatal(err)
	}
	w.Active(nil)
	clock.advance(2 * time.Minute)
	w.releaseIfGone(context.Background())
	if !f.isLoaded("gemma4:26b") {
		t.Fatal("unloaded while the user was still around")
	}
	clock.advance(2 * time.Minute) // no heartbeat for 4 minutes
	w.releaseIfGone(context.Background())
	if f.isLoaded("gemma4:26b") {
		t.Error("model stayed loaded with no presence signal for 4 minutes")
	}
}

func TestBackgroundDoneKeepsTheInteractiveModelWhilePresent(t *testing.T) {
	f, srv := newFakeOllama(t)
	w, pool, _ := newTestWarmer(srv.URL)
	for _, tag := range []string{"translation", "background-generation"} {
		if _, err := pool.AcquireWithTag(tag).Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	w.Active(nil)
	w.BackgroundDone("translation")
	w.BackgroundDone("background-generation")
	if !f.isLoaded("gemma4:26b") {
		t.Error("freed the model the user may want while they are around")
	}
	if f.isLoaded("gemma4:31b") {
		t.Error("background-only model stayed loaded after its run")
	}
}

// busyWith reports whether a warm-up or release is still running.
func (w *Warmer) busyWith() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.busy) > 0
}
