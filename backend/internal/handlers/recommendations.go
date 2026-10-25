package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/models"
	"github.com/rubenwo/mise/internal/recommend"
)

type RecommendationHandler struct {
	queries *database.Queries
	service *recommend.Service
}

func NewRecommendationHandler(q *database.Queries, s *recommend.Service) *RecommendationHandler {
	return &RecommendationHandler{queries: q, service: s}
}

// RecommendedRecipe is one pick with the recipe it refers to.
type RecommendedRecipe struct {
	Recipe models.Recipe  `json:"recipe"`
	Lane   recommend.Lane `json:"lane"`
	Reason string         `json:"reason"`
}

// List returns recipes the user hasn't cooked yet, stable for the day.
// Query: limit (default 12, max 48), exclude (comma-separated recipe IDs),
// offset (skips that many picks, for "show me other ideas").
func (h *RecommendationHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := clampInt(q.Get("limit"), 12, 1, 48)
	offset := clampInt(q.Get("offset"), 0, 0, 200)
	exclude := map[int]bool{}
	for part := range strings.SplitSeq(q.Get("exclude"), ",") {
		if id, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			exclude[id] = true
		}
	}

	picks, err := h.service.Recommend(r.Context(), recommend.Options{
		Limit:        limit + offset,
		Seed:         recommend.DaySeed(time.Now()),
		Exclude:      exclude,
		SimilarShare: 0.25,
	})
	if errors.Is(err, recommend.ErrNoRecipes) {
		writeJSON(w, http.StatusOK, []RecommendedRecipe{})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build recommendations")
		return
	}
	picks = picks[min(offset, len(picks)):]

	recipes, err := h.recipesFor(r, picks)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load recommended recipes")
		return
	}
	writeJSON(w, http.StatusOK, recipes)
}

func (h *RecommendationHandler) recipesFor(r *http.Request, picks []recommend.Pick) ([]RecommendedRecipe, error) {
	ids := make([]int, len(picks))
	for i, p := range picks {
		ids[i] = p.RecipeID
	}
	recipes, err := h.queries.GetRecipesByIDs(r.Context(), ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int]models.Recipe, len(recipes))
	for _, rec := range recipes {
		byID[rec.ID] = rec
	}
	out := make([]RecommendedRecipe, 0, len(picks))
	for _, p := range picks {
		if rec, ok := byID[p.RecipeID]; ok {
			out = append(out, RecommendedRecipe{Recipe: rec, Lane: p.Lane, Reason: p.Reason})
		}
	}
	return out, nil
}

type impressionsRequest struct {
	RecipeIDs []int `json:"recipe_ids"`
}

// Impressions records that recommendations were shown, so ones the user
// keeps passing over fade out.
func (h *RecommendationHandler) Impressions(w http.ResponseWriter, r *http.Request) {
	var req impressionsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.service.RecordImpressions(r.Context(), req.RecipeIDs); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record impressions")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func clampInt(s string, def, lo, hi int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return max(lo, min(hi, n))
}
