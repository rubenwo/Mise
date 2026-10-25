package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/llm"
	"github.com/rubenwo/mise/internal/models"
)

type GenerateHandler struct {
	orchestrator *llm.Orchestrator
	queries      *database.Queries
}

func NewGenerateHandler(o *llm.Orchestrator, q *database.Queries) *GenerateHandler {
	return &GenerateHandler{orchestrator: o, queries: q}
}

// avoidTitlesFromFingerprints returns up to 30 bare titles to seed the avoid
// list passed to the orchestrator. Bare titles cost the fewest tokens; the
// GenerationSystemPrompt helper is the authoritative cap.
func avoidTitlesFromFingerprints(fps []database.RecipeFingerprint) []string {
	n := min(len(fps), 30)
	out := make([]string, n)
	for i := range n {
		out[i] = fps[i].Title
	}
	return out
}

// avoidTitles returns titles the model must not reproduce, favouring the
// target cuisine and recipes pending review. Falls back to the most recent
// library titles if the query fails.
func (h *GenerateHandler) avoidTitles(ctx context.Context, cuisine string, fps []database.RecipeFingerprint) []string {
	titles, err := h.queries.ListRecipeTitles(ctx, cuisine, 40)
	if err != nil {
		log.Printf("Warning: could not fetch avoid titles: %v", err)
		return avoidTitlesFromFingerprints(fps)
	}
	return titles
}

func (h *GenerateHandler) Single(w http.ResponseWriter, r *http.Request) {
	var req models.GenerateRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	fps, err := h.queries.ListRecipeFingerprints(r.Context())
	if err != nil {
		log.Printf("Warning: could not fetch recipe fingerprints: %v", err)
	}
	cuisineCounts, err := h.queries.ListCuisineCounts(r.Context())
	if err != nil {
		log.Printf("Warning: could not fetch cuisine counts: %v", err)
	}

	// Pick the cuisine here rather than in the prompt builder so the avoid
	// list can favour titles from that cuisine.
	if req.CuisineType == "" {
		req.CuisineType = llm.PickLeastRepresentedCuisine(llm.SeedCuisineCounts(cuisineCounts))
	}
	avoidTitles := h.avoidTitles(r.Context(), req.CuisineType, fps)
	prompt := llm.BuildGeneratePrompt(req, cuisineCounts)
	h.streamGeneration(w, r, prompt, avoidTitles, fps)
}

func (h *GenerateHandler) Batch(w http.ResponseWriter, r *http.Request) {
	var req models.BatchGenerateRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.Count <= 0 {
		req.Count = 3
	}
	if req.Count > 10 {
		req.Count = 10
	}

	stream := newSSEStream(w)

	fps, err := h.queries.ListRecipeFingerprints(r.Context())
	if err != nil {
		log.Printf("Warning: could not fetch recipe fingerprints: %v", err)
	}
	cuisineCounts, err := h.queries.ListCuisineCounts(r.Context())
	if err != nil {
		log.Printf("Warning: could not fetch cuisine counts: %v", err)
	}

	// Pre-compute a distinct cuisine for each recipe slot so that concurrent
	// goroutines don't all pick the same least-represented cuisine.
	tempCounts := llm.SeedCuisineCounts(cuisineCounts)
	prompts := make([]string, req.Count)
	avoidTitles := make([][]string, req.Count)
	for i := 0; i < req.Count; i++ {
		slotReq := req.GenerateRequest
		if slotReq.CuisineType == "" {
			slotReq.CuisineType = llm.PickLeastRepresentedCuisine(tempCounts)
			if slotReq.CuisineType != "" {
				tempCounts[slotReq.CuisineType]++
			}
		}
		avoidTitles[i] = h.avoidTitles(r.Context(), slotReq.CuisineType, fps)
		prompts[i] = llm.BuildGeneratePrompt(slotReq, cuisineCounts)
		prompts[i] += fmt.Sprintf(" (Recipe %d of %d — make it unique from others in this batch.)", i+1, req.Count)
	}

	type batchResult struct {
		prompt   string
		recipe   *models.Recipe
		messages []llm.Message
	}

	// Fan-in channel: all goroutines forward their events here.
	allEvents := make(chan llm.SSEEvent, req.Count*10)
	results := make(chan batchResult, req.Count)

	var wg sync.WaitGroup
	for i, p := range prompts {
		wg.Add(1)
		go func(idx int, prompt string) {
			defer wg.Done()
			events := make(chan llm.SSEEvent, 10)

			// Forward this goroutine's events into the shared fan-in channel,
			// tagging each with the slot index so the client can color-code them.
			var fwdWg sync.WaitGroup
			fwdWg.Add(1)
			go func() {
				defer fwdWg.Done()
				for e := range events {
					e.Index = idx
					allEvents <- e
				}
			}()

			recipe, messages, genErr := h.orchestrator.GenerateWithTag(r.Context(), prompt, avoidTitles[idx], events, "generation")
			close(events)
			fwdWg.Wait() // ensure all events are forwarded before signalling done

			if genErr != nil {
				allEvents <- llm.SSEEvent{Type: "error", Message: genErr.Error(), Index: idx}
			}
			results <- batchResult{prompt, recipe, messages}
		}(i, p)
	}

	// Close allEvents once every goroutine is done.
	go func() {
		wg.Wait()
		close(allEvents)
	}()

	// Stream all events to the client. Keep draining after a disconnect so
	// the generation goroutines can finish and their chats get persisted.
	for event := range allEvents {
		sendWithNearDuplicates(stream, event, fps)
	}

	// All goroutines finished; drain results and persist chats.
	for range req.Count {
		res := <-results
		h.saveChat(res.prompt, res.messages)
	}
}

func (h *GenerateHandler) Import(w http.ResponseWriter, r *http.Request) {
	var req models.ImportRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.RawText == "" {
		writeError(w, http.StatusBadRequest, "raw_text is required")
		return
	}

	fps, err := h.queries.ListRecipeFingerprints(r.Context())
	if err != nil {
		log.Printf("Warning: could not fetch recipe fingerprints: %v", err)
	}
	avoidTitles := avoidTitlesFromFingerprints(fps)

	prompt := llm.BuildImportPrompt(req.RawText)
	h.streamGeneration(w, r, prompt, avoidTitles, fps)
}

func (h *GenerateHandler) Refine(w http.ResponseWriter, r *http.Request) {
	var req models.RefineRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.Feedback == "" {
		writeError(w, http.StatusBadRequest, "feedback is required")
		return
	}

	stream := newSSEStream(w)
	prompt := llm.BuildRefinePrompt(req.Recipe, req.Feedback)
	events := make(chan llm.SSEEvent, 10)

	go func() {
		defer close(events)
		_, messages, err := h.orchestrator.GenerateRefine(r.Context(), prompt, events)
		if err != nil {
			log.Printf("Refine error: %v", err)
			events <- llm.SSEEvent{Type: "error", Message: err.Error()}
		}
		h.saveChat(prompt, messages)
	}()

	// Drain until the producer closes the channel, even after a disconnect,
	// so the generation goroutine never blocks on a full buffer.
	for event := range events {
		stream.send(event)
	}
}

func (h *GenerateHandler) streamGeneration(w http.ResponseWriter, r *http.Request, prompt string, avoidTitles []string, fingerprints []database.RecipeFingerprint) {
	stream := newSSEStream(w)
	events := make(chan llm.SSEEvent, 10)

	go func() {
		defer close(events)
		_, messages, err := h.orchestrator.GenerateWithTag(r.Context(), prompt, avoidTitles, events, "generation")
		if err != nil {
			log.Printf("Generation error: %v", err)
			events <- llm.SSEEvent{Type: "error", Message: err.Error()}
		}
		h.saveChat(prompt, messages)
	}()

	for event := range events {
		sendWithNearDuplicates(stream, event, fingerprints)
	}
}

// sendWithNearDuplicates emits near_duplicate warnings before a recipe event
// so the frontend can attach them to the recipe before rendering it.
func sendWithNearDuplicates(stream *sseStream, event llm.SSEEvent, fingerprints []database.RecipeFingerprint) {
	if recipe, ok := event.Data.(models.Recipe); ok && event.Type == "recipe" {
		for _, dup := range findNearDuplicates(recipe, fingerprints) {
			stream.send(llm.SSEEvent{
				Type:    "near_duplicate",
				Index:   event.Index,
				Message: fmt.Sprintf("Similar to existing recipe: %q", dup.Title),
				Data:    map[string]any{"id": dup.ID, "title": dup.Title},
			})
		}
	}
	stream.send(event)
}

func (h *GenerateHandler) saveChat(prompt string, messages []llm.Message) {
	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		log.Printf("Failed to marshal chat messages: %v", err)
		return
	}
	if err := h.queries.CreateGenerationChat(context.Background(), prompt, h.orchestrator.Model(), messagesJSON); err != nil {
		log.Printf("Failed to save generation chat: %v", err)
	}
}
