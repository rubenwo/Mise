package handlers

import (
	"errors"
	"net/http"
	"slices"
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
// Query: limit (default 12, max 96), offset (skips that many picks, for
// "show me other ideas"), exclude (comma-separated recipe IDs), and the
// library filters cuisine, max_minutes and dietary.
func (h *RecommendationHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := clampInt(q.Get("limit"), 12, 1, 96)
	offset := clampInt(q.Get("offset"), 0, 0, 200)
	exclude := map[int]bool{}
	for part := range strings.SplitSeq(q.Get("exclude"), ",") {
		if id, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			exclude[id] = true
		}
	}
	if err := h.excludeFiltered(r, exclude); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to apply filters")
		return
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

// excludeFiltered adds every recipe that fails the request's cuisine,
// max_minutes or dietary filter to exclude.
func (h *RecommendationHandler) excludeFiltered(r *http.Request, exclude map[int]bool) error {
	q := r.URL.Query()
	cuisine := q.Get("cuisine")
	maxMinutes := clampInt(q.Get("max_minutes"), 0, 0, 1000)
	dietary := q.Get("dietary")
	if cuisine == "" && maxMinutes == 0 && dietary == "" {
		return nil
	}
	meta, err := h.queries.ListRecipeFilterMeta(r.Context())
	if err != nil {
		return err
	}
	for _, m := range meta {
		if cuisine != "" && !strings.EqualFold(m.Cuisine, cuisine) ||
			maxMinutes > 0 && (m.Minutes == 0 || m.Minutes > maxMinutes) ||
			dietary != "" && !slices.ContainsFunc(m.Dietary, func(d string) bool { return strings.EqualFold(d, dietary) }) {
			exclude[m.ID] = true
		}
	}
	return nil
}

// Favourites returns recipes rated at least min_rating (default 7, the
// user's "good food" threshold), best first.
func (h *RecommendationHandler) Favourites(w http.ResponseWriter, r *http.Request) {
	minRating, err := strconv.ParseFloat(r.URL.Query().Get("min_rating"), 64)
	if err != nil {
		minRating = 7
	}
	ids, err := h.queries.ListFavouriteRecipeIDs(r.Context(), minRating)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list favourites")
		return
	}
	recipes, err := h.queries.GetRecipesByIDs(r.Context(), ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load favourites")
		return
	}
	byID := make(map[int]models.Recipe, len(recipes))
	for _, rec := range recipes {
		byID[rec.ID] = rec
	}
	out := make([]models.Recipe, 0, len(ids))
	for _, id := range ids {
		if rec, ok := byID[id]; ok {
			out = append(out, rec)
		}
	}
	writeJSON(w, http.StatusOK, out)
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

// Status reports recommender coverage for the settings page.
func (h *RecommendationHandler) Status(w http.ResponseWriter, r *http.Request) {
	st, err := h.service.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build recommendations")
		return
	}
	writeJSON(w, http.StatusOK, st)
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
