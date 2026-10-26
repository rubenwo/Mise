package handlers

import (
	"net/http"

	"github.com/rubenwo/mise/internal/llm"
)

// PresenceHandler receives the frontend's presence signals for the model
// warmer.
type PresenceHandler struct {
	warmer *llm.Warmer
}

func NewPresenceHandler(w *llm.Warmer) *PresenceHandler {
	return &PresenceHandler{warmer: w}
}

// Report records that the user is active (with the features they are likely
// to use right away) or has left. The frontend sends it on navigation, on
// intent (opening the chat) and as a heartbeat while the user is active.
func (h *PresenceHandler) Report(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Active  bool     `json:"active"`
		Intents []string `json:"intents"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Active {
		h.warmer.Active(req.Intents)
	} else {
		h.warmer.Away()
	}
	w.WriteHeader(http.StatusNoContent)
}
