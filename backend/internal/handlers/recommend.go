package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/llm"
	"github.com/rubenwo/mise/internal/models"
)

// recommendExtractPrompt drives the first LLM turn: convert the conversation
// into a single library_search tool call.
const recommendExtractPrompt = `You help the user find recipes from their existing library. Call library_search exactly once to fetch candidates. Extract structured fields from the latest user message, taking earlier turns into account when the user is refining.

Examples:
- "I'm cooking for 8 vegetarians, balanced, nothing exotic" → {"keywords":"","dietary_restrictions":["vegetarian"]}
- "italian pasta dishes under 30 minutes" → {"keywords":"pasta","cuisine_type":"Italian","max_total_minutes":30}
- "quick vegetarian dinner with chickpeas" → {"keywords":"chickpeas","dietary_restrictions":["vegetarian"],"tags":["quick"]}

Prefer broad searches that return many candidates — the ranking step picks the best fit. Use keywords only for distinctive ingredient or dish words; leave it empty if the user described a style rather than a dish.`

// recommendRankPrompt drives the second LLM turn: pick the best N candidates
// from the search results based on the user's stated needs. The candidate list
// is appended to this prompt.
const recommendRankPromptHeader = `You are recommending recipes from the user's library. Pick the %d recipes from the candidate list that best fit the user's request, judging by their description, ingredients, cuisine, dietary fit, and time. If fewer than %d candidates are a good fit, return fewer. Never invent IDs that are not in the candidate list.

Reply with this exact JSON shape (no other text):
{"recipe_ids": [<id>, <id>, ...], "explanation": "<2-3 sentence note covering why these fit>"}

Order recipe_ids from best fit to worst.

Candidates:
`

// rankResponseSchema is the structured reply expected from the ranking turn.
type rankResponseSchema struct {
	RecipeIDs   []int  `json:"recipe_ids"`
	Explanation string `json:"explanation"`
}

// RecommendRequest is the client payload for a recommendation turn.
type RecommendRequest struct {
	Messages []models.ChatMessage `json:"messages"`
	Limit    int                  `json:"limit"`
}

// RecommendResponse is the structured reply for the frontend chat panel.
type RecommendResponse struct {
	Message string          `json:"message"`
	Recipes []models.Recipe `json:"recipes"`
}

// RecommendChat picks recipes from the user's library that match a natural-language
// request. Stateless — the client passes the full conversation each turn.
func (h *RecipeHandler) RecommendChat(w http.ResponseWriter, r *http.Request) {
	var req RecommendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages required")
		return
	}
	last := req.Messages[len(req.Messages)-1]
	if last.Role != "user" || strings.TrimSpace(last.Content) == "" {
		writeError(w, http.StatusBadRequest, "last message must be from user")
		return
	}
	if h.llmPool == nil {
		writeError(w, http.StatusServiceUnavailable, "recommendations not available")
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 3
	}
	if limit > 10 {
		limit = 10
	}

	client := h.llmPool.AcquireWithTag("search")
	if client == nil {
		writeError(w, http.StatusServiceUnavailable, `no provider with tag "search" configured or healthy`)
		return
	}

	// Step 1: ask the model to translate the conversation into a library_search call.
	extractMessages := []llm.Message{{Role: "system", Content: recommendExtractPrompt}}
	for _, m := range req.Messages {
		extractMessages = append(extractMessages, llm.Message{Role: m.Role, Content: m.Content})
	}

	searchReq, err := h.extractSearchParams(r.Context(), client, extractMessages, last.Content)
	if err != nil {
		log.Printf("RecommendChat extract: %v", err)
		writeError(w, http.StatusInternalServerError, "could not interpret request: "+err.Error())
		return
	}

	// Run the search with a wider candidate pool so the ranking step has options.
	const candidatePool = 30
	searchReq.Limit = candidatePool
	candidates, err := h.queries.LibrarySearch(r.Context(), searchReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "library search failed")
		return
	}

	if len(candidates) == 0 {
		writeJSON(w, http.StatusOK, RecommendResponse{
			Message: "I couldn't find any recipes in your library that match. Try broadening the criteria, or tell me what to relax.",
			Recipes: []models.Recipe{},
		})
		return
	}

	// If we have fewer candidates than we need, skip the ranking step — return them all.
	if len(candidates) <= limit {
		writeJSON(w, http.StatusOK, RecommendResponse{
			Message: fmt.Sprintf("Here %s the %d recipe%s from your library that fit.",
				pluralIsAre(len(candidates)), len(candidates), pluralS(len(candidates))),
			Recipes: candidates,
		})
		return
	}

	// Step 2: ask the model to rank the candidates and explain its picks.
	ranked, err := h.rankCandidates(r.Context(), client, req.Messages, candidates, limit)
	if err != nil {
		log.Printf("RecommendChat rank: %v — falling back to first %d candidates", err, limit)
		writeJSON(w, http.StatusOK, RecommendResponse{
			Message: fmt.Sprintf("Here are %d recipes that match. (I couldn't refine the ranking, so these are the most recent matches.)", limit),
			Recipes: candidates[:limit],
		})
		return
	}

	writeJSON(w, http.StatusOK, ranked)
}

// extractSearchParams runs the tool-calling turn and parses the library_search arguments.
// Falls back to a keyword-only search using the latest user message if the model
// fails to call the tool.
func (h *RecipeHandler) extractSearchParams(ctx context.Context, client *llm.Client, messages []llm.Message, latestUserMessage string) (database.LibrarySearchRequest, error) {
	resp, err := client.Chat(ctx, messages, []llm.Tool{llm.LibrarySearchTool})
	if err != nil {
		return database.LibrarySearchRequest{}, fmt.Errorf("chat failed: %w", err)
	}

	var args struct {
		Keywords            string   `json:"keywords"`
		CuisineType         string   `json:"cuisine_type"`
		DietaryRestrictions []string `json:"dietary_restrictions"`
		Tags                []string `json:"tags"`
		MaxTotalMinutes     int      `json:"max_total_minutes"`
	}

	if len(resp.Message.ToolCalls) > 0 {
		tc := resp.Message.ToolCalls[0]
		if err := json.Unmarshal(tc.Function.Arguments, &args); err != nil {
			log.Printf("RecommendChat: tool args parse error: %v, raw: %s", err, string(tc.Function.Arguments))
			args.Keywords = latestUserMessage
		}
	} else {
		log.Printf("RecommendChat: no tool call, falling back to keyword search for %q", latestUserMessage)
		args.Keywords = latestUserMessage
	}

	return database.LibrarySearchRequest{
		Keywords:            args.Keywords,
		CuisineType:         args.CuisineType,
		DietaryRestrictions: args.DietaryRestrictions,
		Tags:                args.Tags,
		MaxTotalMinutes:     args.MaxTotalMinutes,
	}, nil
}

// rankCandidates asks the model to pick `limit` recipes from `candidates` that best
// match the conversation, returning a RecommendResponse ready to send to the client.
func (h *RecipeHandler) rankCandidates(ctx context.Context, client *llm.Client, history []models.ChatMessage, candidates []models.Recipe, limit int) (RecommendResponse, error) {
	rankSystem := fmt.Sprintf(recommendRankPromptHeader, limit, limit) + formatCandidates(candidates)

	rankMessages := []llm.Message{{Role: "system", Content: rankSystem}}
	for _, m := range history {
		rankMessages = append(rankMessages, llm.Message{Role: m.Role, Content: m.Content})
	}

	resp, err := client.ChatJSON(ctx, rankMessages)
	if err != nil {
		return RecommendResponse{}, fmt.Errorf("rank chat failed: %w", err)
	}

	content := strings.TrimSpace(resp.Message.Content)
	if idx := strings.Index(content, "{"); idx >= 0 {
		if end := strings.LastIndex(content, "}"); end >= idx {
			content = content[idx : end+1]
		}
	}
	var parsed rankResponseSchema
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return RecommendResponse{}, fmt.Errorf("parse rank JSON: %w, raw: %s", err, content)
	}

	// Filter to IDs that are in the candidate set, preserve LLM's ordering, dedup.
	candidateByID := make(map[int]models.Recipe, len(candidates))
	for _, c := range candidates {
		candidateByID[c.ID] = c
	}
	seen := map[int]bool{}
	picked := make([]models.Recipe, 0, limit)
	for _, id := range parsed.RecipeIDs {
		if seen[id] {
			continue
		}
		if r, ok := candidateByID[id]; ok {
			picked = append(picked, r)
			seen[id] = true
			if len(picked) >= limit {
				break
			}
		}
	}
	if len(picked) == 0 {
		return RecommendResponse{}, fmt.Errorf("no valid recipe IDs in rank response")
	}

	explanation := strings.TrimSpace(parsed.Explanation)
	if explanation == "" {
		explanation = fmt.Sprintf("Here %s %d recipe%s from your library that fit.",
			pluralIsAre(len(picked)), len(picked), pluralS(len(picked)))
	}
	return RecommendResponse{Message: explanation, Recipes: picked}, nil
}

// formatCandidates renders a compact text block describing each candidate so a
// local LLM can rank them. Ingredient names only (no amounts), descriptions
// truncated. Keeps the prompt small enough for 8k-context models.
func formatCandidates(recipes []models.Recipe) string {
	var sb strings.Builder
	for _, r := range recipes {
		fmt.Fprintf(&sb, "ID %d: %s", r.ID, r.Title)
		if r.CuisineType != "" {
			fmt.Fprintf(&sb, " (%s)", r.CuisineType)
		}
		total := r.PrepTimeMinutes + r.CookTimeMinutes
		if total > 0 {
			fmt.Fprintf(&sb, " · %d min", total)
		}
		if r.Servings > 0 {
			fmt.Fprintf(&sb, " · serves %d", r.Servings)
		}
		sb.WriteString("\n")
		if d := strings.TrimSpace(r.Description); d != "" {
			if len(d) > 200 {
				d = d[:200] + "…"
			}
			fmt.Fprintf(&sb, "  %s\n", d)
		}
		if len(r.DietaryRestrictions) > 0 {
			fmt.Fprintf(&sb, "  dietary: %s\n", strings.Join(r.DietaryRestrictions, ", "))
		}
		if len(r.Tags) > 0 {
			fmt.Fprintf(&sb, "  tags: %s\n", strings.Join(r.Tags, ", "))
		}
		if len(r.Ingredients) > 0 {
			names := make([]string, 0, len(r.Ingredients))
			for _, ing := range r.Ingredients {
				if n := strings.TrimSpace(ing.Name); n != "" {
					names = append(names, n)
				}
			}
			fmt.Fprintf(&sb, "  ingredients: %s\n", strings.Join(names, ", "))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func pluralIsAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
