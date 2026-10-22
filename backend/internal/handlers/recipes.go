package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/llm"
	"github.com/rubenwo/mise/internal/models"
	"github.com/rubenwo/mise/internal/tools"
)

type RecipeHandler struct {
	queries       *database.Queries
	imageSearcher *tools.ImageSearcher
	llmPool       *llm.ClientPool
}

func NewRecipeHandler(q *database.Queries, imageSearcher *tools.ImageSearcher, llmPool *llm.ClientPool) *RecipeHandler {
	return &RecipeHandler{queries: q, imageSearcher: imageSearcher, llmPool: llmPool}
}

func (h *RecipeHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	cuisineType := r.URL.Query().Get("cuisine_type")

	if limit <= 0 {
		limit = 20
	} else if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	recipes, total, err := h.queries.ListRecipes(r.Context(), limit, offset, cuisineType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list recipes")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"recipes": recipes,
		"total":   total,
	})
}

func (h *RecipeHandler) ListCuisines(w http.ResponseWriter, r *http.Request) {
	cuisines, err := h.queries.ListCuisines(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list cuisines")
		return
	}
	if cuisines == nil {
		cuisines = []database.CuisineMeta{}
	}
	writeJSON(w, http.StatusOK, cuisines)
}

func (h *RecipeHandler) LibrarySearchDirect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Keywords            string   `json:"keywords"`
		CuisineType         string   `json:"cuisine_type"`
		DietaryRestrictions []string `json:"dietary_restrictions"`
		Tags                []string `json:"tags"`
		MaxTotalMinutes     int      `json:"max_total_minutes"`
		Limit               int      `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}

	recipes, err := h.queries.LibrarySearch(r.Context(), database.LibrarySearchRequest{
		Keywords:            req.Keywords,
		CuisineType:         req.CuisineType,
		DietaryRestrictions: req.DietaryRestrictions,
		Tags:                req.Tags,
		MaxTotalMinutes:     req.MaxTotalMinutes,
		Limit:               req.Limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search recipes")
		return
	}
	if recipes == nil {
		recipes = []models.Recipe{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recipes": recipes,
		"total":   len(recipes),
	})
}

func (h *RecipeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	recipe, err := h.queries.GetRecipe(r.Context(), id)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "recipe not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get recipe")
		return
	}

	writeJSON(w, http.StatusOK, recipe)
}

func (h *RecipeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var recipe models.Recipe
	if err := json.NewDecoder(r.Body).Decode(&recipe); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if recipe.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	recipe.CuisineType = llm.NormalizeCuisine(recipe.CuisineType)

	if err := h.queries.CreateRecipe(r.Context(), &recipe); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create recipe")
		return
	}

	writeJSON(w, http.StatusCreated, recipe)

	// Fetch and store an image in the background so it is ready when the user next views the recipe.
	if h.imageSearcher != nil {
		go h.fetchAndStoreImage(context.Background(), recipe.ID, recipe.Title)
	}
}

func (h *RecipeHandler) fetchAndStoreImage(ctx context.Context, id int, title string) {
	filename := fmt.Sprintf("recipe-%d", id)
	imageURL, err := h.imageSearcher.SearchAndDownloadRecipeImage(ctx, title, filename)
	if err != nil {
		log.Printf("Auto image fetch for recipe %q: %v", title, err)
		return
	}
	if err := h.queries.SetRecipeImage(ctx, id, imageURL); err != nil {
		log.Printf("Auto image fetch: failed to save image for recipe %q: %v", title, err)
	}
}

type UpdateContentRequest struct {
	Ingredients  []models.Ingredient `json:"ingredients"`
	Instructions []string            `json:"instructions"`
	CuisineType  string              `json:"cuisine_type"`
}

func (h *RecipeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req UpdateContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.queries.UpdateRecipeContent(r.Context(), id, req.Ingredients, req.Instructions, llm.NormalizeCuisine(req.CuisineType)); err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "recipe not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update recipe")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *RecipeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.queries.DeleteRecipe(r.Context(), id); err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "recipe not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete recipe")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *RecipeHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req models.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	recipes, total, err := h.queries.SearchRecipes(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search recipes")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"recipes": recipes,
		"total":   total,
	})
}

// PreviewImage returns an image URL for a recipe title without saving anything.
// Used by the frontend to show images for generated recipes before they are saved.
func (h *RecipeHandler) PreviewImage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	if h.imageSearcher == nil {
		writeError(w, http.StatusServiceUnavailable, "image search not available")
		return
	}

	imageURL, err := h.imageSearcher.SearchRecipeImage(r.Context(), req.Title)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not find image: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"image_url": imageURL})
}

func (h *RecipeHandler) FetchImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	recipe, err := h.queries.GetRecipe(r.Context(), id)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "recipe not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get recipe")
		return
	}

	if h.imageSearcher == nil {
		writeError(w, http.StatusServiceUnavailable, "image search not available")
		return
	}

	filename := fmt.Sprintf("recipe-%d", id)
	imageURL, err := h.imageSearcher.SearchAndDownloadRecipeImage(r.Context(), recipe.Title, filename)
	if err != nil {
		log.Printf("Image search for %q failed: %v", recipe.Title, err)
		writeError(w, http.StatusBadGateway, "could not find an image: "+err.Error())
		return
	}

	if err := h.queries.SetRecipeImage(r.Context(), id, imageURL); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save image url")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"image_url": imageURL})
}

// History returns every plan a recipe has appeared on, with completion data
// and rating. Newest cooked first; never-cooked plans last.
func (h *RecipeHandler) History(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	entries, err := h.queries.GetRecipeHistory(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get recipe history")
		return
	}
	if entries == nil {
		entries = []models.RecipeHistoryEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// EatCounts returns aggregated completion stats across the whole library.
// Frontend caches this once per session for the library-card badges.
func (h *RecipeHandler) EatCounts(w http.ResponseWriter, r *http.Request) {
	stats, err := h.queries.ListRecipeEatCounts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get eat counts")
		return
	}
	if stats == nil {
		stats = []models.RecipeEatStats{}
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *RecipeHandler) Suggestions(w http.ResponseWriter, r *http.Request) {
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count <= 0 {
		count = 3
	}
	if count > 10 {
		count = 10
	}

	meta, err := h.queries.ListRecipeMeta(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load recipes")
		return
	}

	ids := selectSuggestedIDs(meta, count, time.Now())
	recipes, err := h.queries.GetRecipesByIDs(r.Context(), ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load suggestions")
		return
	}

	// Preserve the selection order.
	order := make(map[int]int, len(ids))
	for i, id := range ids {
		order[id] = i
	}
	sort.Slice(recipes, func(i, j int) bool {
		return order[recipes[i].ID] < order[recipes[j].ID]
	})

	writeJSON(w, http.StatusOK, recipes)
}

// FindDuplicates scans all recipes and returns groups of likely duplicates,
// identified by comparing normalised titles and descriptions without an LLM.
func (h *RecipeHandler) FindDuplicates(w http.ResponseWriter, r *http.Request) {
	stubs, err := h.queries.ListRecipesForDedup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list recipes")
		return
	}

	n := len(stubs)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(x, y int) { parent[find(x)] = find(y) }

	const threshold = 0.60
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if recipeSimilarity(stubs[i], stubs[j]) >= threshold {
				union(i, j)
			}
		}
	}

	// Group indices by their root.
	groups := map[int][]int{}
	for i := 0; i < n; i++ {
		root := find(i)
		groups[root] = append(groups[root], i)
	}

	// For each group with ≥2 members, collect full recipe data.
	var dupGroups [][]models.Recipe
	for _, indices := range groups {
		if len(indices) < 2 {
			continue
		}
		ids := make([]int, len(indices))
		for k, idx := range indices {
			ids[k] = stubs[idx].ID
		}
		full, err := h.queries.GetRecipesByIDs(r.Context(), ids)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to fetch duplicate recipes")
			return
		}
		dupGroups = append(dupGroups, full)
	}

	if dupGroups == nil {
		dupGroups = [][]models.Recipe{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": dupGroups})
}
