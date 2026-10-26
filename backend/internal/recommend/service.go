package recommend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/models"
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

// settle is how long the worker waits after a change before acting, so a
// burst (accepting a batch of pending recipes) costs one embed and rebuild.
const settle = 2 * time.Second

// store is the slice of database.Queries the service uses.
type store interface {
	ListRecommendRecipes(ctx context.Context) ([]database.RecommendRecipe, error)
	UpsertRecipeEmbeddings(ctx context.Context, model string, embs []database.RecipeEmbedding) error
	RecommendStateKey(ctx context.Context) (string, error)
	ListRecipeIDs(ctx context.Context) ([]int, error)
	ListRecipeEatCounts(ctx context.Context) ([]models.RecipeEatStats, error)
	ListSkipCounts(ctx context.Context) (map[int]int, error)
	ListImpressionCounts(ctx context.Context) (map[int]int, error)
	RecordImpressions(ctx context.Context, recipeIDs []int) error
}

// Service keeps recipe embeddings current and serves recommendations from a
// cached model. A background worker embeds and rebuilds when recipes change,
// so requests never wait for either (except the very first build).
type Service struct {
	store    store
	embedder Embedder
	embModel string        // name stored with each embedding
	wake     chan struct{} // recipes changed: embed and rebuild
	settle   time.Duration

	buildMu sync.Mutex // one rebuild at a time

	mu       sync.Mutex
	model    *Model
	modelKey string
	builtEmb string // embedding model the cached model was built with
}

// NewService returns a recommender that embeds recipes with e, storing them
// under the model name embModel.
func NewService(q *database.Queries, e Embedder, embModel string) *Service {
	return newService(q, e, embModel)
}

func newService(st store, e Embedder, embModel string) *Service {
	return &Service{store: st, embedder: e, embModel: embModel, wake: make(chan struct{}, 1), settle: settle}
}

// Notify tells the service that recipes changed. It returns at once; the
// worker embeds the changes and rebuilds the model in the background.
func (s *Service) Notify() {
	select {
	case s.wake <- struct{}{}:
	default: // already pending
	}
}

// Start runs the worker: at startup, after every Notify (once changes
// settle), and daily as a safety net, it embeds recipes whose text changed
// and rebuilds the model if the library moved on.
func (s *Service) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			s.update(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-s.wake:
				select {
				case <-ctx.Done():
					return
				case <-time.After(s.settle):
				}
				// Changes during the wait are covered by this pass.
				select {
				case <-s.wake:
				default:
				}
			}
		}
	}()
}

// update embeds stale recipes, then rebuilds the model if anything changed.
func (s *Service) update(ctx context.Context) {
	if err := s.syncEmbeddings(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("recipe embedding sync failed", "err", err)
	}
	if err := s.rebuild(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("recommendation model rebuild failed", "err", err)
	}
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
	recipes, err := s.store.ListRecommendRecipes(ctx)
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
	if err := s.store.UpsertRecipeEmbeddings(ctx, s.embModel, embs); err != nil {
		return err
	}
	slog.Info("embedded recipes for recommendations", "count", len(stale), "model", s.embModel,
		"embed", embedded.Sub(start).Round(time.Millisecond), "store", time.Since(embedded).Round(time.Millisecond))
	return nil
}

// rebuild builds a model from the current library unless the cached one is
// already current, and swaps it in. Requests keep using the old model while
// it runs.
func (s *Service) rebuild(ctx context.Context) error {
	s.buildMu.Lock()
	defer s.buildMu.Unlock()
	key, err := s.store.RecommendStateKey(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	current := s.model != nil && s.modelKey == key
	s.mu.Unlock()
	if current {
		return nil
	}
	rows, err := s.store.ListRecommendRecipes(ctx)
	if err != nil {
		return err
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
	m := Build(recipes, 1)
	s.mu.Lock()
	s.model, s.modelKey, s.builtEmb = m, key, embModel
	s.mu.Unlock()
	slog.Info("built recommendation model", "recipes", len(recipes), "families", m.Families(),
		"embeddings", modelCount[embModel], "took", time.Since(start).Round(time.Millisecond))
	return nil
}

// currentModel returns the cached model and whether the library has changed
// since it was built. Only the first call builds synchronously; after that a
// change wakes the worker and the request carries on with the cached model.
func (s *Service) currentModel(ctx context.Context) (*Model, bool, error) {
	s.mu.Lock()
	m, built := s.model, s.modelKey
	s.mu.Unlock()
	if m == nil {
		if err := s.rebuild(ctx); err != nil {
			return nil, false, err
		}
		s.mu.Lock()
		m, built = s.model, s.modelKey
		s.mu.Unlock()
	}
	key, err := s.store.RecommendStateKey(ctx)
	if err != nil {
		return nil, false, err
	}
	if key != built {
		s.Notify() // a change the save hooks didn't see, e.g. a direct DB edit
		return m, true, nil
	}
	return m, false, nil
}

// ErrNoRecipes is returned when the library is empty.
var ErrNoRecipes = errors.New("no recipes to recommend")

// Recommend returns picks using the user's current cook history.
func (s *Service) Recommend(ctx context.Context, opt Options) ([]Pick, error) {
	m, outdated, err := s.currentModel(ctx)
	if err != nil {
		return nil, err
	}
	if len(m.recipes) == 0 {
		return nil, ErrNoRecipes
	}
	if outdated {
		// The cached model may still hold recipes deleted since; a pick of
		// one would break plan filling.
		ids, err := s.store.ListRecipeIDs(ctx)
		if err != nil {
			return nil, err
		}
		exists := make(map[int]bool, len(ids))
		for _, id := range ids {
			exists[id] = true
		}
		exclude := maps.Clone(opt.Exclude)
		if exclude == nil {
			exclude = map[int]bool{}
		}
		for _, r := range m.recipes {
			if !exists[r.ID] {
				exclude[r.ID] = true
			}
		}
		opt.Exclude = exclude
	}
	fb, err := s.feedback(ctx)
	if err != nil {
		return nil, err
	}
	return m.Recommend(fb, opt), nil
}

func (s *Service) feedback(ctx context.Context) (map[int]Feedback, error) {
	stats, err := s.store.ListRecipeEatCounts(ctx)
	if err != nil {
		return nil, err
	}
	skips, err := s.store.ListSkipCounts(ctx)
	if err != nil {
		return nil, err
	}
	impressions, err := s.store.ListImpressionCounts(ctx)
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
	m, _, err := s.currentModel(ctx)
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
	return s.store.RecordImpressions(ctx, ids)
}

// DaySeed returns a seed that is stable for one calendar day, so picks don't
// reshuffle on every page load.
func DaySeed(t time.Time) uint64 {
	y, m, d := t.Date()
	return uint64(y)*10000 + uint64(m)*100 + uint64(d)
}
