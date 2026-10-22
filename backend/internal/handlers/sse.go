package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rubenwo/mise/internal/llm"
)

// sseStream writes Server-Sent Events to a client.
//
// Once a write fails (typically because the client disconnected) further
// sends become no-ops. Callers should keep ranging over their producer
// channel until it closes so producer goroutines never block on a send.
type sseStream struct {
	w      http.ResponseWriter
	rc     *http.ResponseController
	failed bool
}

func newSSEStream(w http.ResponseWriter) *sseStream {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return &sseStream{w: w, rc: http.NewResponseController(w)}
}

func (s *sseStream) send(event llm.SSEEvent) {
	if s.failed {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", data); err != nil {
		s.failed = true
		return
	}
	if err := s.rc.Flush(); err != nil {
		s.failed = true
	}
}

// disconnected reports whether a previous write to the client failed.
func (s *sseStream) disconnected() bool {
	return s.failed
}
