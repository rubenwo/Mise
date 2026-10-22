package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rubenwo/mise/internal/llm"
)

func TestSSEStream_WritesEvents(t *testing.T) {
	rec := httptest.NewRecorder()
	stream := newSSEStream(rec)
	stream.send(llm.SSEEvent{Type: "status", Message: "hi"})

	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q", got)
	}
	if got, want := rec.Body.String(), "data: {\"type\":\"status\",\"message\":\"hi\"}\n\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if !rec.Flushed {
		t.Error("expected response to be flushed")
	}
}

// brokenWriter simulates a client that has gone away.
type brokenWriter struct {
	httptest.ResponseRecorder
	writes int
}

func (b *brokenWriter) Write([]byte) (int, error) {
	b.writes++
	return 0, errors.New("connection reset")
}

func (b *brokenWriter) Header() http.Header { return http.Header{} }

func TestSSEStream_StopsWritingAfterDisconnect(t *testing.T) {
	w := &brokenWriter{}
	stream := newSSEStream(w)
	for range 5 {
		stream.send(llm.SSEEvent{Type: "status"})
	}
	if !stream.disconnected() {
		t.Error("expected stream to report disconnect")
	}
	if w.writes != 1 {
		t.Errorf("writes after disconnect = %d, want 1", w.writes)
	}
}
