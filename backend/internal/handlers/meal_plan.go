package handlers

import (
	"encoding/json"
	"log"
	"math"
	"math/rand/v2"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/rubenwo/mise/internal/database"
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	plan, err := h.queries.GetMealPlan(r.Context(), id)
	if err != nil {
		if err == pgx.ErrNoRows {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
		titles, _ := h.queries.ListRecipeTitles(r.Context())
		prompt := llm.BuildLeftoverPrompt(req.Ingredients, titles)

		events := make(chan llm.SSEEvent, 10)
		go func() {
			for range events {
				// drain events, we don't need SSE here
			}
		}()

		recipe, _, genErr := h.orchestrator.GenerateWithTag(r.Context(), prompt, events, "generation")
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

	type key struct{ name, unit string }
	agg := map[key]*models.AggregatedIngredient{}
	recipeNames := map[key]map[string]bool{}

	for _, mpr := range plan.Recipes {
		scale := 1.0
		if mpr.Recipe.Servings > 0 {
			scale = float64(mpr.Servings) / float64(mpr.Recipe.Servings)
		}
		for _, ing := range mpr.Recipe.Ingredients {
			normalizedName := normalizeIngredientName(ing.Name)
			k := key{normalizedName, strings.ToLower(ing.Unit)}
			if agg[k] == nil {
				agg[k] = &models.AggregatedIngredient{Name: ing.Name, Unit: ing.Unit}
				recipeNames[k] = map[string]bool{}
			}
			agg[k].Amount += ing.Amount * scale
			recipeNames[k][mpr.Recipe.Title] = true
		}
	}

	result := make([]models.AggregatedIngredient, 0, len(agg))
	for k, v := range agg {
		v.Amount = math.Round(v.Amount*100) / 100
		for title := range recipeNames[k] {
			v.Recipes = append(v.Recipes, title)
		}
		sort.Strings(v.Recipes)
		result = append(result, *v)
	}

	result = consolidateIngredients(result)

	// LLM fallback: merge any items that still share a normalized name after
	// deterministic consolidation (e.g. unknown cross-unit density).
	if h.orchestrator != nil {
		if dupes := findRemainingDuplicates(result); len(dupes) > 0 {
			if merged, err := h.orchestrator.DeduplicateIngredients(r.Context(), dupes); err == nil {
				result = applyLLMDedup(result, dupes, merged)
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
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	result, err := h.getPlanIngredients(r, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get plan")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

type AHOrderResult struct {
	Matched  []AHMatchedIngredient          `json:"matched"`
	NotFound []models.AggregatedIngredient  `json:"not_found"`
}

type AHMatchedIngredient struct {
	Ingredient models.AggregatedIngredient `json:"ingredient"`
	Product    ah.Product                  `json:"product"`
}

func (h *MealPlanHandler) OrderAH(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ingredients, err := h.getPlanIngredients(r, id)
	if err != nil {
		if err == pgx.ErrNoRows {
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

	for i, ing := range ingredients {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			query := name
			if h.translator != nil {
				query = h.translator.Translate(r.Context(), name, "nl")
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

// sizeAdjectives are leading qualifiers that do not change what ingredient to buy.
var sizeAdjectives = map[string]bool{
	"large": true, "small": true, "medium": true, "whole": true, "extra": true,
	"fresh": true, "raw": true, "frozen": true,
}

// ingredientDensity maps a normalized ingredient name to its density in g/mL,
// used to convert between weight and volume for the same ingredient.
var ingredientDensity = map[string]float64{
	"flour":             0.53,
	"all-purpose flour": 0.53,
	"bread flour":       0.56,
	"cake flour":        0.44,
	"whole wheat flour": 0.52,
	"wheat flour":       0.52,
	"sugar":             0.845,
	"granulated sugar":  0.845,
	"brown sugar":       0.72,
	"powdered sugar":    0.56,
	"icing sugar":       0.56,
	"salt":              1.20,
	"butter":            0.91,
	"olive oil":         0.91,
	"oil":               0.91,
	"vegetable oil":     0.91,
	"sunflower oil":     0.91,
	"water":             1.00,
	"milk":              1.03,
	"cream":             0.97,
	"honey":             1.42,
	"rice":              0.75,
	"oat":               0.35,
	"rolled oat":        0.35,
	"cornstarch":        0.61,
	"baking powder":     0.90,
	"baking soda":       1.08,
	"cocoa powder":      0.48,
	"almond flour":      0.44,
}

// normalizeIngredientName strips common leading adjectives, parentheticals,
// trailing modifiers and simple plurals so variant names key together.
func normalizeIngredientName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	// Strip parenthetical notes: "butter (unsalted)" → "butter"
	if idx := strings.Index(name, "("); idx >= 0 {
		name = strings.TrimSpace(name[:idx])
	}
	// Strip after comma: "butter, softened" → "butter"
	if idx := strings.Index(name, ","); idx >= 0 {
		name = strings.TrimSpace(name[:idx])
	}
	// Strip common trailing phrases
	for _, suffix := range []string{" for serving", " for garnish", " for coating", " to taste", " to serve"} {
		name = strings.TrimSuffix(name, suffix)
	}
	// Strip leading size/quality adjectives (only when words remain after stripping)
	words := strings.Fields(name)
	for len(words) > 1 && sizeAdjectives[words[0]] {
		words = words[1:]
	}
	name = strings.Join(words, " ")
	// Normalize simple plurals: "eggs" → "egg", "onions" → "onion"
	// Avoid stripping from words ending in "ss" (e.g. "molasses").
	if len(name) > 3 && strings.HasSuffix(name, "s") && !strings.HasSuffix(name, "ss") {
		name = name[:len(name)-1]
	}
	return name
}

// isCountableUnit returns true for units that represent indivisible whole items.
func isCountableUnit(unit string) bool {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "piece", "pieces", "whole", "count", "pcs", "pc",
		"clove", "cloves", "slice", "slices", "head", "heads",
		"stalk", "stalks", "sprig", "sprigs", "bunch", "bunches",
		"can", "cans", "sheet", "sheets":
		return true
	}
	return false
}

// unitToBase converts an amount+unit to a canonical base unit for aggregation.
// Base units: "g" for weight, "ml" for volume. Other units are returned as-is.
func unitToBase(amount float64, unit string) (float64, string) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "kg":
		return amount * 1000, "g"
	case "oz":
		return amount * 28.35, "g"
	case "lb", "lbs":
		return amount * 453.592, "g"
	case "l", "liter", "litre", "liters", "litres":
		return amount * 1000, "ml"
	case "tsp", "teaspoon", "teaspoons":
		return amount * 4.929, "ml"
	case "tbsp", "tablespoon", "tablespoons":
		return amount * 14.787, "ml"
	case "cup", "cups":
		return amount * 240, "ml"
	default:
		return amount, strings.ToLower(strings.TrimSpace(unit))
	}
}

// baseToDisplay converts a base-unit amount to a human-friendly unit.
func baseToDisplay(amount float64, baseUnit string) (float64, string) {
	switch baseUnit {
	case "g":
		if amount >= 1000 {
			return amount / 1000, "kg"
		}
		return amount, "g"
	case "ml":
		if amount >= 1000 {
			return amount / 1000, "l"
		}
		return amount, "ml"
	default:
		return amount, baseUnit
	}
}

// roundAmount rounds to a whole number for amounts >= 10, otherwise 2 decimal places.
func roundAmount(amount float64) float64 {
	if amount >= 10 {
		return math.Round(amount)
	}
	return math.Round(amount*100) / 100
}

// consolidateIngredients merges duplicate ingredients by:
//  1. normalizing names and converting to base units
//  2. re-aggregating by {normName, baseUnit}
//  3. cross-unit merging g↔ml using known ingredient densities
//  4. rounding countable items up to the nearest whole number
func consolidateIngredients(ingredients []models.AggregatedIngredient) []models.AggregatedIngredient {
	type aggKey struct{ name, unit string }
	type entry struct {
		normName string
		amount   float64
		baseUnit string
		recipes  map[string]bool
	}

	// Pass 1: normalize + base-unit convert, aggregate by {normName, baseUnit}.
	agg := map[aggKey]*entry{}
	for _, ing := range ingredients {
		normName := normalizeIngredientName(ing.Name)
		baseAmount, baseUnit := unitToBase(ing.Amount, ing.Unit)
		k := aggKey{normName, baseUnit}
		if agg[k] == nil {
			agg[k] = &entry{normName: normName, baseUnit: baseUnit, recipes: map[string]bool{}}
		}
		agg[k].amount += baseAmount
		for _, r := range ing.Recipes {
			agg[k].recipes[r] = true
		}
	}

	// Pass 2: group by normName; attempt g↔ml cross-unit merge via density table.
	byName := map[string][]*entry{}
	for _, e := range agg {
		byName[e.normName] = append(byName[e.normName], e)
	}

	var merged []*entry
	for _, group := range byName {
		if len(group) == 1 {
			merged = append(merged, group[0])
			continue
		}
		// Separate into g, ml, and other-unit buckets.
		var gAmt, mlAmt float64
		var gRecipes, mlRecipes []string
		hasG, hasML := false, false
		var others []*entry
		for _, e := range group {
			switch e.baseUnit {
			case "g":
				gAmt += e.amount
				for r := range e.recipes {
					gRecipes = append(gRecipes, r)
				}
				hasG = true
			case "ml":
				mlAmt += e.amount
				for r := range e.recipes {
					mlRecipes = append(mlRecipes, r)
				}
				hasML = true
			default:
				others = append(others, e)
			}
		}
		if hasG && hasML {
			density, known := ingredientDensity[group[0].normName]
			if known {
				// Convert ml → g and merge into single g entry.
				combined := &entry{
					normName: group[0].normName,
					baseUnit: "g",
					amount:   gAmt + mlAmt*density,
					recipes:  map[string]bool{},
				}
				for _, r := range gRecipes {
					combined.recipes[r] = true
				}
				for _, r := range mlRecipes {
					combined.recipes[r] = true
				}
				merged = append(merged, combined)
				merged = append(merged, others...)
				continue
			}
		}
		// Cannot merge — leave all entries separate (LLM fallback will handle them).
		merged = append(merged, group...)
	}

	// Pass 3: build output with display units and countable ceiling.
	result := make([]models.AggregatedIngredient, 0, len(merged))
	for _, e := range merged {
		amount, unit := baseToDisplay(e.amount, e.baseUnit)
		if isCountableUnit(unit) {
			amount = math.Ceil(amount)
		} else {
			amount = roundAmount(amount)
		}
		displayName := e.normName
		if len(displayName) > 0 {
			displayName = strings.ToUpper(displayName[:1]) + displayName[1:]
		}
		recipes := make([]string, 0, len(e.recipes))
		for r := range e.recipes {
			recipes = append(recipes, r)
		}
		sort.Strings(recipes)
		result = append(result, models.AggregatedIngredient{
			Name:    displayName,
			Amount:  amount,
			Unit:    unit,
			Recipes: recipes,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

// findRemainingDuplicates returns groups (≥2 items) that still share the same
// normalized name after deterministic consolidation.
func findRemainingDuplicates(ingredients []models.AggregatedIngredient) [][]models.AggregatedIngredient {
	groups := map[string][]int{}
	for i, ing := range ingredients {
		groups[normalizeIngredientName(ing.Name)] = append(groups[normalizeIngredientName(ing.Name)], i)
	}
	var result [][]models.AggregatedIngredient
	for _, indices := range groups {
		if len(indices) < 2 {
			continue
		}
		group := make([]models.AggregatedIngredient, len(indices))
		for j, idx := range indices {
			group[j] = ingredients[idx]
		}
		result = append(result, group)
	}
	return result
}

// applyLLMDedup replaces the duplicate groups in ingredients with the LLM-merged items.
func applyLLMDedup(ingredients []models.AggregatedIngredient, dupes [][]models.AggregatedIngredient, merged []models.AggregatedIngredient) []models.AggregatedIngredient {
	remove := map[string]bool{}
	for _, group := range dupes {
		for _, ing := range group {
			remove[normalizeIngredientName(ing.Name)] = true
		}
	}
	result := make([]models.AggregatedIngredient, 0, len(ingredients))
	for _, ing := range ingredients {
		if !remove[normalizeIngredientName(ing.Name)] {
			result = append(result, ing)
		}
	}
	result = append(result, merged...)
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func (h *MealPlanHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.queries.DeleteMealPlan(r.Context(), id); err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete plan")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *MealPlanHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req models.UpdateMealPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Status != nil {
		valid := map[string]bool{"draft": true, "active": true, "completed": true}
		if !valid[*req.Status] {
			writeError(w, http.StatusBadRequest, "status must be draft, active, or completed")
			return
		}
		if err := h.queries.UpdateMealPlanStatus(r.Context(), id, *req.Status); err != nil {
			if err == pgx.ErrNoRows {
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
	planID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}

	var req models.AddPlanRecipeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
	planID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}

	var req models.RandomizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Servings) == 0 {
		writeError(w, http.StatusBadRequest, "servings array is required (one entry per day)")
		return
	}
	count := len(req.Servings)

	// Fetch all recipes (lightweight) and eaten recipe IDs
	summaries, err := h.queries.ListRecipeSummaries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list recipes")
		return
	}
	if len(summaries) == 0 {
		writeError(w, http.StatusBadRequest, "no recipes in the library to pick from")
		return
	}

	eaten, err := h.queries.ListEatenRecipeIDs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check eaten recipes")
		return
	}

	// Split into new (never eaten) and eaten pools
	var newPool, eatenPool []database.RecipeSummary
	for _, s := range summaries {
		if eaten[s.ID] {
			eatenPool = append(eatenPool, s)
		} else {
			newPool = append(newPool, s)
		}
	}

	// Determine targets: ~50/50, adjusting if a pool is too small
	newTarget := count / 2
	eatenTarget := count - newTarget
	if newTarget > len(newPool) {
		newTarget = len(newPool)
		eatenTarget = count - newTarget
	}
	if eatenTarget > len(eatenPool) {
		eatenTarget = len(eatenPool)
		newTarget = count - eatenTarget
	}
	// Final clamp in case total library is smaller than count
	if newTarget > len(newPool) {
		newTarget = len(newPool)
	}

	selected := selectDiverse(newPool, newTarget)
	selected = append(selected, selectDiverse(eatenPool, eatenTarget)...)

	// Shuffle the final selection so new and eaten are interleaved
	rand.Shuffle(len(selected), func(i, j int) {
		selected[i], selected[j] = selected[j], selected[i]
	})

	recipeIDs := make([]int, len(selected))
	for i, s := range selected {
		recipeIDs[i] = s.ID
	}

	if err := h.queries.ReplacePlanRecipes(r.Context(), planID, recipeIDs, req.Servings); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set plan recipes")
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

// selectDiverse picks n recipes from pool, preferring distinct cuisines.
func selectDiverse(pool []database.RecipeSummary, n int) []database.RecipeSummary {
	if n <= 0 {
		return nil
	}
	if n >= len(pool) {
		result := make([]database.RecipeSummary, len(pool))
		copy(result, pool)
		rand.Shuffle(len(result), func(i, j int) {
			result[i], result[j] = result[j], result[i]
		})
		return result
	}

	// Shuffle the pool for randomness
	shuffled := make([]database.RecipeSummary, len(pool))
	copy(shuffled, pool)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	// Greedily pick recipes, preferring cuisines not yet selected
	usedCuisines := map[string]int{}
	var selected []database.RecipeSummary

	for len(selected) < n {
		bestIdx := -1
		bestCount := math.MaxInt
		for i, s := range shuffled {
			c := usedCuisines[s.CuisineType]
			if c < bestCount {
				bestCount = c
				bestIdx = i
			}
		}
		pick := shuffled[bestIdx]
		selected = append(selected, pick)
		usedCuisines[pick.CuisineType]++
		shuffled = append(shuffled[:bestIdx], shuffled[bestIdx+1:]...)
	}

	return selected
}

func (h *MealPlanHandler) RemoveRecipe(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	recipeID, err := strconv.Atoi(chi.URLParam(r, "recipeId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid recipe id")
		return
	}

	if err := h.queries.RemoveRecipeFromPlan(r.Context(), planID, recipeID); err != nil {
		if err == pgx.ErrNoRows {
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
	planID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	recipeID, err := strconv.Atoi(chi.URLParam(r, "recipeId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid recipe id")
		return
	}

	var req models.UpdatePlanRecipeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.queries.UpdatePlanRecipe(r.Context(), planID, recipeID, req.Servings, req.Completed); err != nil {
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
