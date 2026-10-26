package recommend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/llm"
)

// EmbeddingTag is the provider tag that serves recipe embeddings.
const EmbeddingTag = "embedding"

// embedBatch recipes go in one request. Large batches matter because the
// model is unloaded after every request (keep_alive 0).
const embedBatch = 256

// Service keeps recipe embeddings current and serves recommendations from a
// cached model that is rebuilt whenever the library changes.
type Service struct {
	queries *database.Queries
	pool    *llm.ClientPool
	sync    chan struct{} // requests an embedding sync

	mu       sync.Mutex
	model    *Model
	modelKey string
	embModel string // embedding model the cached model was built with
}

func NewService(q *database.Queries, pool *llm.ClientPool) *Service {
	return &Service{queries: q, pool: pool, sync: make(chan struct{}, 1)}
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

// embedPrefix is the task instruction a model expects for clustering-style
// similarity. On the recipe benchmark EmbeddingGemma's clustering prompt
// improved flavour structure (AUC 0.933 → 0.953) at no cost to duplicate
// detection; models without a documented prompt get none.
func embedPrefix(model string) string {
	switch {
	case strings.HasPrefix(model, "embeddinggemma"):
		return "task: clustering | query: "
	case strings.HasPrefix(model, "nomic-embed-text"):
		return "clustering: "
	}
	return ""
}

func embedText(model string, r database.RecommendRecipe) string {
	return embedPrefix(model) + fmt.Sprintf("%s. %s Cuisine: %s. Ingredients: %s.",
		r.Title, r.Description, r.Cuisine, strings.Join(r.Ingredients, ", "))
}

func textHash(model, text string) string {
	sum := sha256.Sum256([]byte(model + "\x00" + text))
	return hex.EncodeToString(sum[:8])
}

// syncEmbeddings embeds every recipe whose text or model changed since it
// was last embedded. Without an embedding provider it does nothing and the
// recommender runs on ingredient TF-IDF alone.
func (s *Service) syncEmbeddings(ctx context.Context) error {
	client := s.pool.AcquireWithTag(EmbeddingTag)
	if client == nil {
		return nil
	}
	recipes, err := s.queries.ListRecommendRecipes(ctx)
	if err != nil {
		return err
	}
	var stale []database.RecommendRecipe
	for _, r := range recipes {
		if r.TextHash != textHash(client.Model(), embedText(client.Model(), r)) {
			stale = append(stale, r)
		}
	}
	for start := 0; start < len(stale); start += embedBatch {
		batch := stale[start:min(start+embedBatch, len(stale))]
		texts := make([]string, len(batch))
		for i, r := range batch {
			texts[i] = embedText(client.Model(), r)
		}
		vecs, err := client.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("embedding %d recipes: %w", len(batch), err)
		}
		for i, r := range batch {
			if err := s.queries.UpsertRecipeEmbedding(ctx, r.ID, client.Model(), textHash(client.Model(), texts[i]), vecs[i]); err != nil {
				return err
			}
		}
	}
	if len(stale) > 0 {
		slog.Info("embedded recipes for recommendations", "count", len(stale), "model", client.Model())
	}
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
	s.embModel = embModel

	// Recipes added or edited since their last embedding: sync them in the
	// background; the sync changes the state key, so the next request
	// rebuilds with the fresh embeddings.
	if client := s.pool.AcquireWithTag(EmbeddingTag); client != nil {
		for _, r := range rows {
			if r.TextHash != textHash(client.Model(), embedText(client.Model(), r)) {
				select {
				case s.sync <- struct{}{}:
				default: // a sync is already pending
				}
				break
			}
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
}

// Status reports how many recipes the model covers, how many of them have
// embeddings and how many flavour families it found.
func (s *Service) Status(ctx context.Context) (Status, error) {
	m, err := s.currentModel(ctx)
	if err != nil {
		return Status{}, err
	}
	st := Status{Recipes: len(m.recipes), Families: m.Families()}
	for _, e := range m.emb {
		if e != nil {
			st.Embedded++
		}
	}
	s.mu.Lock()
	st.Model = s.embModel
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
