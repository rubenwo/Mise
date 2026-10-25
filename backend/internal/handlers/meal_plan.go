package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/ingredients"
	"github.com/rubenwo/mise/internal/integrations/ah"
	"github.com/rubenwo/mise/internal/llm"
	"github.com/rubenwo/mise/internal/models"
	"github.com/rubenwo/mise/internal/translation"
)

type MealPlanHandler struct {
	queries      *database.Queries
	orchestrator *llm.Orchestrator
	ahClient     *ah.Client
	translator   *translation.Translator
}

func NewMealPlanHandler(q *database.Queries, o *llm.Orchestrator, searchTimeout time.Duration, t *translation.Translator) *MealPlanHandler {
	return &MealPlanHandler{queries: q, orchestrator: o, ahClient: ah.NewClient(searchTimeout), translator: t}
}

func (h *MealPlanHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	plan, err := h.queries.CreateMealPlan(r.Context(), req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create plan")
		return
	}

	writeJSON(w, http.StatusCreated, plan)
}

func (h *MealPlanHandler) List(w http.ResponseWriter, r *http.Request) {
	plans, err := h.queries.ListMealPlans(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list plans")
		return
	}

	writeJSON(w, http.StatusOK, plans)
}

func (h *MealPlanHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "invalid id")
	if !ok {
		return
	}

	plan, err := h.queries.GetMealPlan(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get plan")
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

type SuggestionsRequest struct {
	Ingredients []string `json:"ingredients"`
}

type SuggestionsResponse struct {
	DBRecipes       []models.Recipe `json:"db_recipes"`
	GeneratedRecipe *models.Recipe  `json:"generated_recipe,omitempty"`
}

func (h *MealPlanHandler) Suggestions(w http.ResponseWriter, r *http.Request) {
	var req SuggestionsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Ingredients) == 0 {
		writeError(w, http.StatusBadRequest, "ingredients are required")
		return
	}

	// Search DB first
	dbRecipes, err := h.queries.SearchRecipesByIngredients(r.Context(), req.Ingredients)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search recipes")
		return
	}

	resp := SuggestionsResponse{DBRecipes: dbRecipes}

	// If fewer than 2 DB matches, generate via LLM
	if len(dbRecipes) < 2 && h.orchestrator != nil {
		titles, _ := h.queries.ListRecipeTitles(r.Context(), "", 40)
		prompt := llm.BuildLeftoverPrompt(req.Ingredients)

		events := make(chan llm.SSEEvent, 10)
		go func() {
			for range events {
				// drain events, we don't need SSE here
			}
		}()

		recipe, _, genErr := h.orchestrator.GenerateWithTag(r.Context(), prompt, titles, events, "generation")
		if genErr == nil && recipe != nil {
			resp.GeneratedRecipe = recipe
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// getPlanIngredients returns the aggregated ingredient list for a plan,
// using the cache when available.
func (h *MealPlanHandler) getPlanIngredients(r *http.Request, id int) ([]models.AggregatedIngredient, error) {
	if cached, err := h.queries.GetPlanNormalizedIngredients(r.Context(), id); err == nil && len(cached) > 0 {
		var result []models.AggregatedIngredient
		if err := json.Unmarshal(cached, &result); err == nil {
			return result, nil
		}
	}

	plan, err := h.queries.GetMealPlan(r.Context(), id)
	if err != nil {
		return nil, err
	}

	result := ingredients.Aggregate(plan)

	// LLM fallback: merge any items that still share a normalized name after
	// deterministic consolidation (e.g. unknown cross-unit density).
	if h.orchestrator != nil {
		if dupes := ingredients.FindRemainingDuplicates(result); len(dupes) > 0 {
			if merged, err := h.orchestrator.DeduplicateIngredients(r.Context(), dupes); err == nil {
				result = ingredients.ApplyMerged(result, dupes, merged)
			} else {
				log.Printf("LLM ingredient dedup failed: %v", err)
			}
		}
	}

	if cacheJSON, err := json.Marshal(result); err == nil {
		_ = h.queries.SetPlanNormalizedIngredients(r.Context(), id, cacheJSON)
	}

	return result, nil
}

func (h *MealPlanHandler) Ingredients(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "invalid id")
	if !ok {
		return
	}

	result, err := h.getPlanIngredients(r, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get plan")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

type AHOrderResult struct {
	Matched  []AHMatchedIngredient         `json:"matched"`
	NotFound []models.AggregatedIngredient `json:"not_found"`
}

type AHMatchedIngredient struct {
	Ingredient models.AggregatedIngredient `json:"ingredient"`
	Product    ah.Product                  `json:"product"`
}

func (h *MealPlanHandler) OrderAH(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "invalid id")
	if !ok {
		return
	}
	ingredients, err := h.getPlanIngredients(r, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get plan ingredients")
		return
	}
	if len(ingredients) == 0 {
		writeJSON(w, http.StatusOK, AHOrderResult{Matched: []AHMatchedIngredient{}, NotFound: []models.AggregatedIngredient{}})
		return
	}

	// Search AH for each ingredient in parallel (max 5 concurrent requests).
	type result struct {
		idx     int
		product *ah.Product
		err     error
	}

	sem := make(chan struct{}, 5)
	results := make([]result, len(ingredients))
	var wg sync.WaitGroup

	// Translation populates a DB cache, which is valuable even if the user
	// navigates away mid-request. Use a detached context with its own timeout
	// so cancelled requests still finish their cache-warming work.
	translateCtx, cancelTranslate := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelTranslate()

	for i, ing := range ingredients {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			query := name
			if h.translator != nil {
				query = h.translator.Translate(translateCtx, name, "nl")
			}
			p, err := h.ahClient.SearchProduct(query)
			results[idx] = result{idx: idx, product: p, err: err}
		}(i, ing.Name)
	}
	wg.Wait()

	var matched []AHMatchedIngredient
	var notFound []models.AggregatedIngredient

	for i, res := range results {
		if res.err != nil || res.product == nil {
			notFound = append(notFound, ingredients[i])
		} else {
			matched = append(matched, AHMatchedIngredient{
				Ingredient: ingredients[i],
				Product:    *res.product,
			})
		}
	}

	if matched == nil {
		matched = []AHMatchedIngredient{}
	}
	if notFound == nil {
		notFound = []models.AggregatedIngredient{}
	}

	writeJSON(w, http.StatusOK, AHOrderResult{Matched: matched, NotFound: notFound})
}

func (h *MealPlanHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "invalid id")
	if !ok {
		return
	}

	if err := h.queries.DeleteMealPlan(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete plan")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *MealPlanHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "invalid id")
	if !ok {
		return
	}

	var req models.UpdateMealPlanRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.Status != nil {
		valid := map[string]bool{"draft": true, "active": true, "completed": true}
		if !valid[*req.Status] {
			writeError(w, http.StatusBadRequest, "status must be draft, active, or completed")
			return
		}
		if err := h.queries.UpdateMealPlanStatus(r.Context(), id, *req.Status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "plan not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to update plan")
			return
		}
	}

	plan, err := h.queries.GetMealPlan(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get updated plan")
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

func (h *MealPlanHandler) AddRecipe(w http.ResponseWriter, r *http.Request) {
	planID, ok := pathID(w, r, "id", "invalid plan id")
	if !ok {
		return
	}

	var req models.AddPlanRecipeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RecipeID == 0 {
		writeError(w, http.StatusBadRequest, "recipe_id is required")
		return
	}
	if req.Servings <= 0 {
		req.Servings = 4
	}

	if err := h.queries.AddRecipeToPlan(r.Context(), planID, req.RecipeID, req.Servings); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add recipe to plan")
		return
	}
	_ = h.queries.InvalidatePlanIngredients(r.Context(), planID)

	plan, err := h.queries.GetMealPlan(r.Context(), planID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get updated plan")
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

func (h *MealPlanHandler) Randomize(w http.ResponseWriter, r *http.Request) {
	planID, ok := pathID(w, r, "id", "invalid plan id")
	if !ok {
		return
	}

	var req models.RandomizeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Servings) == 0 {
		writeError(w, http.StatusBadRequest, "servings array is required (one entry per day)")
		return
	}
	total := len(req.Servings)

	currentPlan, err := h.queries.GetMealPlan(r.Context(), planID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load current plan")
		return
	}
	existingCount := len(currentPlan.Recipes)
	needed := total - existingCount
	if needed <= 0 {
		writeJSON(w, http.StatusOK, currentPlan)
		return
	}

	existingIDs := make(map[int]bool, existingCount)
	for _, mpr := range currentPlan.Recipes {
		existingIDs[mpr.RecipeID] = true
	}

	summaries, err := h.queries.ListRecipeSummaries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list recipes")
		return
	}
	if len(summaries) == 0 {
		writeError(w, http.StatusBadRequest, "no recipes in the library to pick from")
		return
	}

	everPlaced, err := h.queries.ListEatenRecipeIDs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check eaten recipes")
		return
	}

	eatStats, err := h.queries.ListRecipeEatCounts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch recipe stats")
		return
	}
	statsByID := make(map[int]models.RecipeEatStats, len(eatStats))
	for _, s := range eatStats {
		statsByID[s.RecipeID] = s
	}

	excludedIDs := make(map[int]bool, len(req.ExcludedIDs))
	for _, id := range req.ExcludedIDs {
		excludedIDs[id] = true
	}

	// Build the primary pool (everything not already in this plan and not user-excluded)
	// and a fallback "excluded" pool used only if the primary pool runs dry.
	var pool, fallback []database.RecipeSummary
	for _, s := range summaries {
		if existingIDs[s.ID] {
			continue
		}
		if excludedIDs[s.ID] {
			fallback = append(fallback, s)
			continue
		}
		pool = append(pool, s)
	}

	now := time.Now()
	weightFor := func(s database.RecipeSummary) float64 {
		w := 1.0
		st, hasStats := statsByID[s.ID]
		if hasStats && st.AvgRating != nil {
			// 5★ -> 1.0, 10★ -> 2.0, 1★ -> 0.2. Clamp to avoid zero weight.
			r := *st.AvgRating / 5.0
			if r < 0.1 {
				r = 0.1
			}
			w *= r
		}
		if hasStats && st.LastCookedAt != nil {
			age := now.Sub(*st.LastCookedAt)
			switch {
			case age < 14*24*time.Hour:
				w /= 3.0
			case age < 30*24*time.Hour:
				w /= 1.5
			}
		}
		if hasStats && st.Count > 0 {
			// Soft frequency penalty so well-loved recipes don't dominate forever.
			// Cooked 3×→÷1.3, 10×→÷1.85, capped at ÷3.0.
			divisor := 1.0 + 0.1*float64(st.Count)
			if divisor > 3.0 {
				divisor = 3.0
			}
			w /= divisor
		}
		if !everPlaced[s.ID] {
			w *= 1.3
		}
		return w
	}

	selected := weightedSample(pool, needed, weightFor)
	if stillNeeded := needed - len(selected); stillNeeded > 0 {
		selected = append(selected, weightedSample(fallback, stillNeeded, weightFor)...)
	}

	selected = diversifyCuisines(selected, pool, weightFor)

	defaultServings := req.Servings[len(req.Servings)-1]
	for i, s := range selected {
		servings := defaultServings
		idx := existingCount + i
		if idx < len(req.Servings) {
			servings = req.Servings[idx]
		}
		if err := h.queries.AddRecipeToPlan(r.Context(), planID, s.ID, servings); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to add recipe to plan")
			return
		}
	}
	_ = h.queries.InvalidatePlanIngredients(r.Context(), planID)

	plan, err := h.queries.GetMealPlan(r.Context(), planID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get updated plan")
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

func (h *MealPlanHandler) RemoveRecipe(w http.ResponseWriter, r *http.Request) {
	planID, ok := pathID(w, r, "id", "invalid plan id")
	if !ok {
		return
	}
	recipeID, ok := pathID(w, r, "recipeId", "invalid recipe id")
	if !ok {
		return
	}

	if err := h.queries.RemoveRecipeFromPlan(r.Context(), planID, recipeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "recipe not in plan")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to remove recipe")
		return
	}
	_ = h.queries.InvalidatePlanIngredients(r.Context(), planID)

	plan, err := h.queries.GetMealPlan(r.Context(), planID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get updated plan")
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

func (h *MealPlanHandler) UpdateRecipe(w http.ResponseWriter, r *http.Request) {
	planID, ok := pathID(w, r, "id", "invalid plan id")
	if !ok {
		return
	}
	recipeID, ok := pathID(w, r, "recipeId", "invalid recipe id")
	if !ok {
		return
	}

	var req models.UpdatePlanRecipeRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	if err := h.queries.UpdatePlanRecipe(r.Context(), planID, recipeID, req.Servings, req.Completed, req.Skipped, req.Rating); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update recipe in plan")
		return
	}
	// Invalidate ingredient cache when servings change (not for completed toggle).
	if req.Servings != nil {
		_ = h.queries.InvalidatePlanIngredients(r.Context(), planID)
	}

	plan, err := h.queries.GetMealPlan(r.Context(), planID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get updated plan")
		return
	}

	writeJSON(w, http.StatusOK, plan)
}
