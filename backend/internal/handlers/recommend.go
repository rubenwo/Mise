package handlers

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/rubenwo/mise/internal/llm"
	"github.com/rubenwo/mise/internal/models"
)

// Finding recipes for a request like "on holiday, want quick home-made
// meals" runs in three steps. The model reads the request and works out what
// the user's situation implies (every ingredient has to be bought, so few of
// them) as filters plus a one-line description of the ideal dish. Code then
// applies the filters to the whole library, relaxing the soft ones if nothing
// is left, and orders what remains by how well its embedding matches the
// description. Finally the model reads the best dozen in full (ingredients
// and method, so it can judge cookware and effort) and picks the ones to show.

// libraryQueryPrompt is sent as the user turn: gemma4 follows instructions
// there more reliably than in a system prompt.
const libraryQueryPrompt = `Turn a request for recipe ideas into search filters for the user's own recipe library.

%s

First, in situation, think about what the user's circumstances imply for the dish beyond their literal words. Someone on holiday or without a stocked kitchen has to buy every ingredient, so they want few ingredients and simple cooking; someone tired or short on time wants something quick; cooking for a crowd calls for dishes that scale. When the latest message refines an earlier one, keep the earlier requirements unless the user drops them.

Then fill in the filters from what the user said and what their situation implies, leaving anything that doesn't apply empty or 0. Put a diet in dietary only when the dish must suit it. Use cuisine for a single cuisine they named and max_minutes for a time limit (about 30 for quick). Include and avoid hold only foods you could buy, such as chickpeas or mushrooms, never flavours, dish types or cooking styles: include what they want in the dish, and avoid only foods the user names as unwanted or can't eat. A wish like "lighter" or "less heavy" belongs in the description, not in avoid. If they need few ingredients, set max_ingredients to the most they would accept (about 10). Finally, write description as one short line describing the ideal dish, including whatever the filters can't capture, such as texture, cookware, effort, occasion or mood.`

const libraryRankPrompt = `The user is looking for recipe ideas from their own library.

%s

What their situation implies: %s

These are the closest matches in the library:

%s
Pick up to %d recipes that best fit everything the user asked for, best first. Judge cookware, effort and the number of ingredients from the method and ingredient list. Leave out recipes that clearly break a requirement, and if none fits every requirement, pick the closest and say what doesn't fit. Write the explanation in two or three sentences to the user, mentioning the picked recipes by name (never by ID). Only use IDs from the list.`

// shortlistSize is how many candidates the ranking turn reads in full.
const shortlistSize = 12

// libraryQuery is the model's reading of a request (LibraryQuerySchema).
type libraryQuery struct {
	Situation      string   `json:"situation"`
	Dietary        []string `json:"dietary"`
	Cuisine        string   `json:"cuisine"`
	MaxMinutes     int      `json:"max_minutes"`
	Include        []string `json:"include"`
	Avoid          []string `json:"avoid"`
	MaxIngredients int      `json:"max_ingredients"`
	Description    string   `json:"description"`
}

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
	if !decodeJSON(w, r, &req) {
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
	limit := min(max(req.Limit, 1), 10)
	if req.Limit <= 0 {
		limit = 3
	}

	client := h.llmPool.AcquireWithTag("search")
	if client == nil {
		writeError(w, http.StatusServiceUnavailable, `no provider with tag "search" configured or healthy`)
		return
	}

	conversation := formatConversation(req.Messages)
	query, err := readLibraryQuery(r.Context(), client, conversation)
	if err != nil {
		log.Printf("RecommendChat read request: %v", err)
		writeError(w, http.StatusInternalServerError, "could not interpret request: "+err.Error())
		return
	}

	recipes, _, err := h.queries.ListRecipes(r.Context(), 100000, 0, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "library search failed")
		return
	}
	var scores map[int]float64
	if h.searcher != nil {
		if scores, err = h.searcher.Search(r.Context(), query.Description); err != nil {
			log.Printf("RecommendChat semantic search: %v (ranking without it)", err)
		}
	}
	candidates, relaxed := shortlist(recipes, query, scores, shortlistSize)
	if len(candidates) == 0 {
		writeJSON(w, http.StatusOK, RecommendResponse{
			Message: "I couldn't find any recipes in your library that match. Try broadening the criteria, or tell me what to relax.",
			Recipes: []models.Recipe{},
		})
		return
	}
	note := relaxedNote(relaxed)

	ranked, err := rankCandidates(r.Context(), client, conversation, query.Situation, candidates, limit)
	if err != nil {
		log.Printf("RecommendChat rank: %v — falling back to the %d closest matches", err, limit)
		ranked = RecommendResponse{
			Message: "Here are the closest matches in your library. (I couldn't weigh them against your request, so check the details.)",
			Recipes: candidates[:min(limit, len(candidates))],
		}
	}
	if note != "" {
		ranked.Message = note + " " + ranked.Message
	}
	writeJSON(w, http.StatusOK, ranked)
}

// formatConversation renders the chat for the model, keeping earlier turns
// so refinements ("something lighter") build on the original request.
func formatConversation(messages []models.ChatMessage) string {
	last := messages[len(messages)-1].Content
	if len(messages) == 1 {
		return fmt.Sprintf("The user's request: %q", last)
	}
	var sb strings.Builder
	sb.WriteString("Earlier in the conversation:\n")
	for _, m := range messages[:len(messages)-1] {
		who := "User"
		if m.Role != "user" {
			who = "Mise"
		}
		fmt.Fprintf(&sb, "%s: %s\n", who, strings.TrimSpace(m.Content))
	}
	fmt.Fprintf(&sb, "\nThe user's latest message: %q", last)
	return sb.String()
}

func readLibraryQuery(ctx context.Context, client *llm.Client, conversation string) (libraryQuery, error) {
	resp, err := client.ChatJSON(ctx, []llm.Message{{Role: "user", Content: fmt.Sprintf(libraryQueryPrompt, conversation)}}, llm.LibraryQuerySchema)
	if err != nil {
		return libraryQuery{}, err
	}
	var q libraryQuery
	if err := json.Unmarshal([]byte(extractJSONObject(resp.Message.Content)), &q); err != nil {
		return libraryQuery{}, fmt.Errorf("parse request: %w", err)
	}
	return q, nil
}

func extractJSONObject(content string) string {
	content = strings.TrimSpace(content)
	if i := strings.Index(content, "{"); i >= 0 {
		if j := strings.LastIndex(content, "}"); j >= i {
			return content[i : j+1]
		}
	}
	return content
}

// softFilter is a requirement that is dropped when no recipe meets it on
// top of the ones already applied. Diets and avoided ingredients are hard
// filters and never dropped.
type softFilter struct {
	note string // what was given up, for the reply
	keep func(r *models.Recipe) bool
}

// shortlist applies q to the library and returns the n best candidates, plus
// a note for each soft requirement that had to be dropped to find any.
// Candidates are ordered by semantic match with q.Description, minus a
// penalty per ingredient over the requested maximum.
func shortlist(recipes []models.Recipe, q libraryQuery, scores map[int]float64, n int) ([]models.Recipe, []string) {
	var pool []*models.Recipe
	for i := range recipes {
		r := &recipes[i]
		if !slices.ContainsFunc(q.Dietary, func(d string) bool { return !llm.SuitsDiet(r, d) }) &&
			!slices.ContainsFunc(q.Avoid, func(a string) bool { return mentions(r, a) }) {
			pool = append(pool, r)
		}
	}

	// In priority order: wanted ingredients and cuisine are usually stated
	// outright, a time limit is often inferred ("quick").
	var soft []softFilter
	for _, inc := range q.Include {
		soft = append(soft, softFilter{inc, func(r *models.Recipe) bool { return mentions(r, inc) }})
	}
	if q.Cuisine != "" {
		soft = append(soft, softFilter{"the " + q.Cuisine + " cuisine", func(r *models.Recipe) bool {
			return strings.EqualFold(r.CuisineType, q.Cuisine)
		}})
	}
	if q.MaxMinutes > 0 {
		soft = append(soft, softFilter{fmt.Sprintf("the %d-minute limit", q.MaxMinutes), func(r *models.Recipe) bool {
			return r.PrepTimeMinutes+r.CookTimeMinutes <= q.MaxMinutes
		}})
	}

	var relaxed []string
	matches := pool
	for _, f := range soft {
		kept := slices.DeleteFunc(slices.Clone(matches), func(r *models.Recipe) bool { return !f.keep(r) })
		if len(kept) == 0 {
			relaxed = append(relaxed, f.note)
			continue
		}
		matches = kept
	}

	score := func(r *models.Recipe) float64 {
		s := scores[r.ID]
		if q.MaxIngredients > 0 {
			s -= 0.02 * float64(max(0, len(r.Ingredients)-q.MaxIngredients))
		}
		return s
	}
	slices.SortStableFunc(matches, func(a, b *models.Recipe) int { return cmp.Compare(score(b), score(a)) })
	out := make([]models.Recipe, 0, min(n, len(matches)))
	for _, r := range matches[:min(n, len(matches))] {
		out = append(out, *r)
	}
	return out, relaxed
}

// foodKinds lists what counts as a general food word, which recipes rarely
// use themselves ("spaghetti", not "pasta").
var foodKinds = map[string][]string{
	"pasta":  {"spaghetti", "penne", "linguine", "tagliatelle", "fusilli", "rigatoni", "macaroni", "orzo", "lasagn", "pappardelle", "farfalle", "orecchiette", "gnocchi", "ravioli", "tortellini"},
	"noodle": {"udon", "ramen", "soba", "vermicelli", "glass noodle", "rice noodle", "egg noodle", "japchae"},
	"fish":   {"salmon", "cod", "tuna", "haddock", "halibut", "tilapia", "trout", "mackerel", "sea bass", "sardine", "anchov", "pollock", "hake", "snapper", "sole"},
}

// mentions reports whether a recipe's title or ingredients name food
// ("mushrooms" matches "chestnut mushroom", "pasta" matches "penne").
func mentions(r *models.Recipe, food string) bool {
	food = strings.ToLower(strings.TrimSpace(food))
	if kinds, ok := foodKinds[strings.TrimSuffix(food, "s")]; ok &&
		slices.ContainsFunc(kinds, func(k string) bool { return mentionsWord(r, k) }) {
		return true
	}
	return mentionsWord(r, food)
}

func mentionsWord(r *models.Recipe, food string) bool {
	food = strings.ToLower(strings.TrimSpace(food))
	if food == "" {
		return false
	}
	stem := food
	switch {
	case strings.HasSuffix(stem, "oes"): // tomatoes, potatoes
		stem = strings.TrimSuffix(stem, "es")
	case len(stem) > 3 && strings.HasSuffix(stem, "s"): // eggs, olives, chickpeas
		stem = strings.TrimSuffix(stem, "s")
	}
	if strings.Contains(strings.ToLower(r.Title), stem) {
		return true
	}
	return slices.ContainsFunc(r.Ingredients, func(i models.Ingredient) bool {
		// Fish sauce seasons half of South-East Asia; it doesn't make a fish dish.
		name := strings.ReplaceAll(strings.ToLower(i.Name), "fish sauce", "")
		return strings.Contains(name, stem)
	})
}

func relaxedNote(relaxed []string) string {
	switch len(relaxed) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("Nothing in your library met every requirement, so I let go of %s.", relaxed[0])
	}
	return fmt.Sprintf("Nothing in your library met every requirement, so I let go of %s and %s.",
		strings.Join(relaxed[:len(relaxed)-1], ", "), relaxed[len(relaxed)-1])
}

// rankCandidates has the model read the shortlist in full and pick up to
// limit recipes, returning them in its order with its explanation.
func rankCandidates(ctx context.Context, client *llm.Client, conversation, situation string, candidates []models.Recipe, limit int) (RecommendResponse, error) {
	prompt := fmt.Sprintf(libraryRankPrompt, conversation, situation, formatCandidates(candidates), limit)
	resp, err := client.ChatJSON(ctx, []llm.Message{{Role: "user", Content: prompt}}, llm.RankSchema)
	if err != nil {
		return RecommendResponse{}, fmt.Errorf("rank chat failed: %w", err)
	}
	var parsed rankResponseSchema
	content := extractJSONObject(resp.Message.Content)
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return RecommendResponse{}, fmt.Errorf("parse rank JSON: %w, raw: %s", err, content)
	}

	byID := make(map[int]models.Recipe, len(candidates))
	for _, c := range candidates {
		byID[c.ID] = c
	}
	picked := make([]models.Recipe, 0, limit)
	for _, id := range parsed.RecipeIDs {
		if r, ok := byID[id]; ok && len(picked) < limit {
			picked = append(picked, r)
			delete(byID, id) // drop duplicates
		}
	}
	explanation := strings.TrimSpace(parsed.Explanation)
	if len(picked) == 0 && explanation == "" {
		return RecommendResponse{}, fmt.Errorf("no valid recipe IDs in rank response")
	}
	if explanation == "" {
		explanation = fmt.Sprintf("Here %s %d recipe%s from your library that fit.",
			pluralIsAre(len(picked)), len(picked), pluralS(len(picked)))
	}
	return RecommendResponse{Message: explanation, Recipes: picked}, nil
}

// formatCandidates renders each candidate with what the ranking needs to
// judge it: time, ingredient count and names, and the method (for cookware
// and effort).
func formatCandidates(recipes []models.Recipe) string {
	var sb strings.Builder
	for _, r := range recipes {
		fmt.Fprintf(&sb, "ID %d: %s (%s, %d min, %d ingredients)\n", r.ID, r.Title, r.CuisineType,
			r.PrepTimeMinutes+r.CookTimeMinutes, len(r.Ingredients))
		names := make([]string, 0, len(r.Ingredients))
		for _, ing := range r.Ingredients {
			if n := strings.TrimSpace(ing.Name); n != "" {
				names = append(names, n)
			}
		}
		fmt.Fprintf(&sb, "Ingredients: %s\n", strings.Join(names, ", "))
		method := strings.Join(r.Instructions, " ")
		if runes := []rune(method); len(runes) > 900 {
			method = string(runes[:900]) + "…"
		}
		fmt.Fprintf(&sb, "Method: %s\n\n", method)
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
