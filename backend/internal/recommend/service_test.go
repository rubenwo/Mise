package recommend

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/models"
)

// fakeStore is an in-memory library whose state key changes on every write.
type fakeStore struct {
	mu      sync.Mutex
	recipes []database.RecommendRecipe
	version int
	gate    chan struct{} // when set, ListRecommendRecipes waits on it
}

func newFakeStore(groups, perGroup int) *fakeStore {
	st := &fakeStore{}
	for _, r := range library(groups, perGroup) {
		st.recipes = append(st.recipes, database.RecommendRecipe{
			ID: r.ID, Title: r.Title, Cuisine: r.Cuisine, Ingredients: r.Ingredients, Minutes: r.Minutes,
		})
	}
	return st
}

func (f *fakeStore) ListRecommendRecipes(context.Context) ([]database.RecommendRecipe, error) {
	f.mu.Lock()
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.recipes), nil
}

func (f *fakeStore) UpsertRecipeEmbeddings(_ context.Context, model string, embs []database.RecipeEmbedding) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range embs {
		for i := range f.recipes {
			if f.recipes[i].ID == e.RecipeID {
				f.recipes[i].EmbeddingModel, f.recipes[i].TextHash, f.recipes[i].Embedding = model, e.TextHash, e.Vector
			}
		}
	}
	f.version++
	return nil
}

func (f *fakeStore) RecommendStateKey(context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fmt.Sprint(f.version), nil
}

func (f *fakeStore) ListRecipeIDs(context.Context) ([]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []int
	for _, r := range f.recipes {
		ids = append(ids, r.ID)
	}
	return ids, nil
}

func (f *fakeStore) ListRecipeEatCounts(context.Context) ([]models.RecipeEatStats, error) {
	return nil, nil
}
func (f *fakeStore) ListSkipCounts(context.Context) (map[int]int, error)       { return nil, nil }
func (f *fakeStore) ListImpressionCounts(context.Context) (map[int]int, error) { return nil, nil }
func (f *fakeStore) RecordImpressions(context.Context, []int) error            { return nil }

func (f *fakeStore) add(r database.RecommendRecipe) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recipes = append(f.recipes, r)
	f.version++
}

func (f *fakeStore) remove(ids ...int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recipes = slices.DeleteFunc(f.recipes, func(r database.RecommendRecipe) bool { return slices.Contains(ids, r.ID) })
	f.version++
}

// fakeEmbedder returns a deterministic vector per text and records calls.
type fakeEmbedder struct {
	mu    sync.Mutex
	calls [][]string
}

func (e *fakeEmbedder) Available() bool { return true }

func (e *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	e.calls = append(e.calls, texts)
	e.mu.Unlock()
	out := make([][]float32, len(texts))
	for i, t := range texts {
		h := fnv.New64a()
		h.Write([]byte(t))
		rng := rand.New(rand.NewPCG(h.Sum64(), 1))
		v := make([]float32, 16)
		for d := range v {
			v[d] = float32(rng.NormFloat64())
		}
		out[i] = v
	}
	return out, nil
}

func (e *fakeEmbedder) embedded() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var all []string
	for _, c := range e.calls {
		all = append(all, c...)
	}
	return all
}

func startService(t *testing.T, st *fakeStore, emb *fakeEmbedder) *Service {
	t.Helper()
	s := newService(st, emb, "fake-model")
	s.settle = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.Start(ctx)
	return s
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func status(t *testing.T, s *Service) Status {
	t.Helper()
	st, err := s.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestNotifyEmbedsNewRecipeAndRebuilds(t *testing.T) {
	st := newFakeStore(12, 8)
	emb := &fakeEmbedder{}
	s := startService(t, st, emb)
	waitFor(t, "startup embedding", func() bool { return status(t, s).Embedded == 96 })

	before := len(emb.embedded())
	st.add(database.RecommendRecipe{ID: 1000, Title: "Chicken Laksa", Cuisine: "Malaysian", Ingredients: []string{"laksa paste"}})
	s.Notify()
	waitFor(t, "new recipe embedded and in the model", func() bool {
		got := status(t, s)
		return got.Recipes == 97 && got.Embedded == 97
	})
	if added := emb.embedded()[before:]; len(added) != 1 {
		t.Errorf("re-embedded %d texts for one new recipe", len(added))
	}
}

func TestRequestsUseCachedModelDuringRebuild(t *testing.T) {
	st := newFakeStore(12, 8)
	s := startService(t, st, &fakeEmbedder{})
	waitFor(t, "startup embedding", func() bool { return status(t, s).Embedded == 96 })

	// Hold the worker inside its next pass, then change the library.
	gate := make(chan struct{})
	st.mu.Lock()
	st.gate = gate
	st.mu.Unlock()
	st.add(database.RecommendRecipe{ID: 1000, Title: "Chicken Laksa", Cuisine: "Malaysian", Ingredients: []string{"laksa paste"}})
	s.Notify()

	done := make(chan error, 1)
	go func() {
		_, err := s.Recommend(context.Background(), Options{Limit: 5, Seed: 1})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Recommend waited for the background rebuild")
	}
	if got := status(t, s).Recipes; got != 96 {
		t.Errorf("model has %d recipes before the rebuild finished, want the cached 96", got)
	}

	st.mu.Lock()
	st.gate = nil
	st.mu.Unlock()
	close(gate)
	waitFor(t, "rebuild with the new recipe", func() bool { return status(t, s).Recipes == 97 })
}

func TestOutdatedModelNeverPicksDeletedRecipes(t *testing.T) {
	st := newFakeStore(12, 8)
	s := newService(st, &fakeEmbedder{}, "fake-model") // no worker: the model stays outdated
	if _, err := s.Recommend(context.Background(), Options{Limit: 1, Seed: 1}); err != nil {
		t.Fatal(err)
	}
	var deleted []int
	for id := 1; id <= 96; id += 2 {
		deleted = append(deleted, id)
	}
	st.remove(deleted...)

	picks, err := s.Recommend(context.Background(), Options{Limit: 40, Seed: 3, Exclude: map[int]bool{2: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) == 0 {
		t.Fatal("no picks from the outdated model")
	}
	for _, p := range picks {
		if slices.Contains(deleted, p.RecipeID) || p.RecipeID == 2 {
			t.Errorf("picked deleted or excluded recipe %d", p.RecipeID)
		}
	}
}
