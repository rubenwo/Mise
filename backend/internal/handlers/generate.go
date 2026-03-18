package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/rubenwoldhuis/recipes/internal/llm"
	"github.com/rubenwoldhuis/recipes/internal/models"
)

type GenerateHandler struct {
	orchestrator *llm.Orchestrator
}

func NewGenerateHandler(o *llm.Orchestrator) *GenerateHandler {
	return &GenerateHandler{orchestrator: o}
}

func (h *GenerateHandler) Single(w http.ResponseWriter, r *http.Request) {
	var req models.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	prompt := llm.BuildGeneratePrompt(req)
	h.streamGeneration(w, r, prompt)
}

func (h *GenerateHandler) Batch(w http.ResponseWriter, r *http.Request) {
	var req models.BatchGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Count <= 0 {
		req.Count = 3
	}
	if req.Count > 10 {
		req.Count = 10
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for i := 0; i < req.Count; i++ {
		events := make(chan llm.SSEEvent, 10)

		go func() {
			defer close(events)
			prompt := llm.BuildGeneratePrompt(req.GenerateRequest)
			prompt += fmt.Sprintf(" (Recipe %d of %d — make it unique from others in this batch)", i+1, req.Count)
			_, err := h.orchestrator.Generate(r.Context(), prompt, events)
			if err != nil {
				events <- llm.SSEEvent{Type: "error", Message: err.Error()}
			}
		}()

		for event := range events {
			data, _ := json.Marshal(event)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *GenerateHandler) Refine(w http.ResponseWriter, r *http.Request) {
	var req models.RefineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Feedback == "" {
		writeError(w, http.StatusBadRequest, "feedback is required")
		return
	}

	prompt := llm.BuildRefinePrompt(req.Recipe, req.Feedback)
	h.streamGeneration(w, r, prompt)
}

func (h *GenerateHandler) streamGeneration(w http.ResponseWriter, r *http.Request, prompt string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events := make(chan llm.SSEEvent, 10)

	go func() {
		defer close(events)
		_, err := h.orchestrator.Generate(r.Context(), prompt, events)
		if err != nil {
			log.Printf("Generation error: %v", err)
			events <- llm.SSEEvent{Type: "error", Message: err.Error()}
		}
	}()

	for event := range events {
		data, _ := json.Marshal(event)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return
		}
		flusher.Flush()
	}
}
