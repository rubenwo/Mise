package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/rubenwoldhuis/recipes/internal/database"
	"github.com/rubenwoldhuis/recipes/internal/llm"
	"github.com/rubenwoldhuis/recipes/internal/models"
)

type MealPlanHandler struct {
	queries      *database.Queries
	orchestrator *llm.Orchestrator
}

func NewMealPlanHandler(q *database.Queries, o *llm.Orchestrator) *MealPlanHandler {
	return &MealPlanHandler{queries: q, orchestrator: o}
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

		recipe, _, genErr := h.orchestrator.Generate(r.Context(), prompt, events)
		if genErr == nil && recipe != nil {
			resp.GeneratedRecipe = recipe
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

type AggregatedIngredient struct {
	Name    string   `json:"name"`
	Amount  float64  `json:"amount"`
	Unit    string   `json:"unit"`
	Recipes []string `json:"recipes"`
}

func (h *MealPlanHandler) Ingredients(w http.ResponseWriter, r *http.Request) {
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

	type key struct{ name, unit string }
	agg := map[key]*AggregatedIngredient{}
	recipeNames := map[key]map[string]bool{}

	for _, mpr := range plan.Recipes {
		scale := 1.0
		if mpr.Recipe.Servings > 0 {
			scale = float64(mpr.Servings) / float64(mpr.Recipe.Servings)
		}
		for _, ing := range mpr.Recipe.Ingredients {
			k := key{strings.ToLower(ing.Name), strings.ToLower(ing.Unit)}
			if agg[k] == nil {
				agg[k] = &AggregatedIngredient{Name: ing.Name, Unit: ing.Unit}
				recipeNames[k] = map[string]bool{}
			}
			agg[k].Amount += ing.Amount * scale
			recipeNames[k][mpr.Recipe.Title] = true
		}
	}

	result := make([]AggregatedIngredient, 0, len(agg))
	for k, v := range agg {
		v.Amount = math.Round(v.Amount*100) / 100
		for title := range recipeNames[k] {
			v.Recipes = append(v.Recipes, title)
		}
		sort.Strings(v.Recipes)
		result = append(result, *v)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	writeJSON(w, http.StatusOK, result)
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

	plan, err := h.queries.GetMealPlan(r.Context(), planID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get updated plan")
		return
	}

	writeJSON(w, http.StatusOK, plan)
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

	plan, err := h.queries.GetMealPlan(r.Context(), planID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get updated plan")
		return
	}

	writeJSON(w, http.StatusOK, plan)
}
