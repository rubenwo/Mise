package recommend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rubenwo/mise/internal/database"
)

// Embedder turns recipe texts into unit-length vectors. The recommender's
// blend weight and same-dish percentiles were measured on EmbeddingGemma, so
// the server always runs that model (package embedding) rather than a
// configurable one.
type Embedder interface {
	Available() bool
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// embedPrompt is EmbeddingGemma's clustering task prompt. On the recipe
// benchmark it improved flavour structure (AUC 0.933 → 0.953) at no cost to
// duplicate detection.
const embedPrompt = "task: clustering | query: "

// Service keeps recipe embeddings current and serves recommendations from a
// cached model that is rebuilt whenever the library changes.
type Service struct {
	queries  *database.Queries
	embedder Embedder
	embModel string        // name stored with each embedding
	sync     chan struct{} // requests an embedding sync

	mu       sync.Mutex
	model    *Model
	modelKey string
	builtEmb string // embedding model the cached model was built with
}

// NewService returns a recommender that embeds recipes with e, storing them
// under the model name embModel.
func NewService(q *database.Queries, e Embedder, embModel string) *Service {
	return &Service{queries: q, embedder: e, embModel: embModel, sync: make(chan struct{}, 1)}
}

// Start embeds new or edited recipes at startup and whenever a model rebuild
// finds recipes without a current embedding (see currentModel), with a daily
// sweep as a safety net. A new recipe is recommendable straight away on
// TF-IDF and gains its embedding moments later.
func (s *Service) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			if err := s.syncEmbeddings(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("recipe embedding sync failed", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-s.sync:
			case <-ticker.C:
			}
		}
	}()
}

func embedText(r database.RecommendRecipe) string {
	return embedPrompt + fmt.Sprintf("%s. %s Cuisine: %s. Ingredients: %s.",
		r.Title, r.Description, r.Cuisine, strings.Join(r.Ingredients, ", "))
}

// stale reports whether r lacks an embedding of its current text.
func (s *Service) stale(r database.RecommendRecipe) bool {
	return r.TextHash != textHash(s.embModel, embedText(r))
}

func textHash(model, text string) string {
	sum := sha256.Sum256([]byte(model + "\x00" + text))
	return hex.EncodeToString(sum[:8])
}

// syncEmbeddings embeds every recipe whose text or model changed since it
// was last embedded. Without the model files it does nothing and the
// recommender runs on ingredient TF-IDF alone.
func (s *Service) syncEmbeddings(ctx context.Context) error {
	if !s.embedder.Available() {
		return nil
	}
	recipes, err := s.queries.ListRecommendRecipes(ctx)
	if err != nil {
		return err
	}
	var stale []database.RecommendRecipe
	var texts []string
	for _, r := range recipes {
		if s.stale(r) {
			stale = append(stale, r)
			texts = append(texts, embedText(r))
		}
	}
	if len(stale) == 0 {
		return nil
	}
	start := time.Now()
	vecs, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embedding %d recipes: %w", len(stale), err)
	}
	embedded := time.Now()
	embs := make([]database.RecipeEmbedding, len(stale))
	for i, r := range stale {
		embs[i] = database.RecipeEmbedding{RecipeID: r.ID, TextHash: textHash(s.embModel, texts[i]), Vector: vecs[i]}
	}
	if err := s.queries.UpsertRecipeEmbeddings(ctx, s.embModel, embs); err != nil {
		return err
	}
	slog.Info("embedded recipes for recommendations", "count", len(stale), "model", s.embModel,
		"embed", embedded.Sub(start).Round(time.Millisecond), "store", time.Since(embedded).Round(time.Millisecond))
	return nil
}

// currentModel returns the cached model, rebuilding it if the library or
// embeddings changed since it was built.
func (s *Service) currentModel(ctx context.Context) (*Model, error) {
	key, err := s.queries.RecommendStateKey(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model != nil && s.modelKey == key {
		return s.model, nil
	}
	rows, err := s.queries.ListRecommendRecipes(ctx)
	if err != nil {
		return nil, err
	}
	// Mixed models would make embeddings incomparable; use the most common one.
	modelCount := map[string]int{}
	for _, r := range rows {
		if r.EmbeddingModel != "" {
			modelCount[r.EmbeddingModel]++
		}
	}
	embModel := ""
	for m, n := range modelCount {
		if n > modelCount[embModel] {
			embModel = m
		}
	}
	recipes := make([]Recipe, len(rows))
	for i, r := range rows {
		recipes[i] = Recipe{ID: r.ID, Title: r.Title, Cuisine: r.Cuisine, Tags: r.Tags, Ingredients: r.Ingredients, Minutes: r.Minutes}
		if r.EmbeddingModel == embModel {
			recipes[i].Embedding = r.Embedding
		}
	}
	start := time.Now()
	s.model = Build(recipes, 1)
	s.modelKey = key
	s.builtEmb = embModel

	// Recipes added or edited since their last embedding: sync them in the
	// background; the sync changes the state key, so the next request
	// rebuilds with the fresh embeddings.
	if s.embedder.Available() && slices.ContainsFunc(rows, s.stale) {
		select {
		case s.sync <- struct{}{}:
		default: // a sync is already pending
		}
	}
	slog.Info("built recommendation model", "recipes", len(recipes), "families", s.model.Families(),
		"embeddings", modelCount[embModel], "took", time.Since(start).Round(time.Millisecond))
	return s.model, nil
}

// ErrNoRecipes is returned when the library is empty.
var ErrNoRecipes = errors.New("no recipes to recommend")

// Recommend returns picks using the user's current cook history.
func (s *Service) Recommend(ctx context.Context, opt Options) ([]Pick, error) {
	m, err := s.currentModel(ctx)
	if err != nil {
		return nil, err
	}
	if len(m.recipes) == 0 {
		return nil, ErrNoRecipes
	}
	fb, err := s.feedback(ctx)
	if err != nil {
		return nil, err
	}
	return m.Recommend(fb, opt), nil
}

func (s *Service) feedback(ctx context.Context) (map[int]Feedback, error) {
	stats, err := s.queries.ListRecipeEatCounts(ctx)
	if err != nil {
		return nil, err
	}
	skips, err := s.queries.ListSkipCounts(ctx)
	if err != nil {
		return nil, err
	}
	impressions, err := s.queries.ListImpressionCounts(ctx)
	if err != nil {
		return nil, err
	}
	fb := map[int]Feedback{}
	for _, st := range stats {
		fb[st.RecipeID] = Feedback{CookCount: st.Count, AvgRating: st.AvgRating}
	}
	for id, n := range skips {
		f := fb[id]
		f.Skipped = n
		fb[id] = f
	}
	for id, n := range impressions {
		f := fb[id]
		f.Impressions = n
		fb[id] = f
	}
	return fb, nil
}

// Status describes the recommender for the settings page.
type Status struct {
	Recipes  int    `json:"recipes"`
	Embedded int    `json:"embedded"`
	Model    string `json:"embedding_model"`
	Families int    `json:"families"`
	// EmbedderReady is false when the image lacks the model files.
	EmbedderReady bool `json:"embedder_ready"`
}

// Status reports how many recipes the model covers, how many of them have
// embeddings and how many flavour families it found.
func (s *Service) Status(ctx context.Context) (Status, error) {
	m, err := s.currentModel(ctx)
	if err != nil {
		return Status{}, err
	}
	st := Status{Recipes: len(m.recipes), Families: m.Families(), EmbedderReady: s.embedder.Available()}
	for _, e := range m.emb {
		if e != nil {
			st.Embedded++
		}
	}
	s.mu.Lock()
	st.Model = s.builtEmb
	s.mu.Unlock()
	return st, nil
}

// RecordImpressions notes that the given recipes were shown to the user today.
func (s *Service) RecordImpressions(ctx context.Context, ids []int) error {
	return s.queries.RecordImpressions(ctx, ids)
}

// DaySeed returns a seed that is stable for one calendar day, so picks don't
// reshuffle on every page load.
func DaySeed(t time.Time) uint64 {
	y, m, d := t.Date()
	return uint64(y)*10000 + uint64(m)*100 + uint64(d)
}
