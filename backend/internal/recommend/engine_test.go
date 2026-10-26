package recommend

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

// library builds groups of recipes that share ingredients within a group and
// nothing across groups, so the right families are unambiguous.
func library(groups, perGroup int) []Recipe {
	var rs []Recipe
	id := 1
	for g := range groups {
		for i := range perGroup {
			rs = append(rs, Recipe{
				ID:      id,
				Title:   fmt.Sprintf("Dish %d-%d", g, i),
				Cuisine: fmt.Sprintf("Cuisine%d", g),
				Ingredients: []string{
					fmt.Sprintf("base%d", g), fmt.Sprintf("spice%d", g), fmt.Sprintf("herb%d", g),
					fmt.Sprintf("extra%d-%d", g, i%3),
				},
				Minutes: 30,
			})
			id++
		}
	}
	return rs
}

func rating(r float64) *float64 { return new(r) }

func TestSphericalKMeansRecoversSeparatedGroups(t *testing.T) {
	recipes := library(4, 12)
	vecs, _ := tfidf(recipes)
	c := kmeansOnce(vecs, 4, rand.New(rand.NewPCG(1, 2)))
	for g := range 4 {
		first := c.assign[g*12]
		for i := 1; i < 12; i++ {
			if c.assign[g*12+i] != first {
				t.Fatalf("group %d split across families: %v", g, c.assign[g*12:g*12+12])
			}
		}
	}
}

func TestRecommendSkipsCookedExcludedAndSameDish(t *testing.T) {
	recipes := library(12, 8)
	m := Build(recipes, 1)
	fb := map[int]Feedback{1: {CookCount: 2, AvgRating: rating(10)}, 9: {CookCount: 1, AvgRating: rating(5)}}
	exclude := map[int]bool{2: true, 3: true}
	picks := m.Recommend(fb, Options{Limit: 12, Seed: 7, Exclude: exclude, SimilarShare: 0.25})
	if len(picks) != 12 {
		t.Fatalf("got %d picks, want 12", len(picks))
	}
	seen := map[int]bool{}
	reasons := map[string]bool{}
	for _, p := range picks {
		if p.Lane == LaneSimilar {
			if reasons[p.Reason] {
				t.Errorf("two similar picks share the reason %q", p.Reason)
			}
			reasons[p.Reason] = true
		}
		if p.RecipeID == 1 || p.RecipeID == 9 || exclude[p.RecipeID] {
			t.Errorf("picked cooked or excluded recipe %d", p.RecipeID)
		}
		if seen[p.RecipeID] {
			t.Errorf("recipe %d picked twice", p.RecipeID)
		}
		seen[p.RecipeID] = true
		if p.Reason == "" {
			t.Errorf("pick %d has no reason", p.RecipeID)
		}
	}
}

func TestRecommendSpreadsAcrossFamilies(t *testing.T) {
	m := Build(library(12, 8), 1)
	picks := m.Recommend(nil, Options{Limit: 8, Seed: 3})
	families := map[int]bool{}
	for _, p := range picks {
		families[m.families.assign[m.index[p.RecipeID]]] = true
	}
	if len(families) != len(picks) {
		t.Errorf("%d picks came from only %d families", len(picks), len(families))
	}
}

func TestRecommendIsDeterministicPerSeedAndRotates(t *testing.T) {
	m := Build(library(12, 8), 1)
	ids := func(seed uint64) []int {
		var out []int
		for _, p := range m.Recommend(nil, Options{Limit: 8, Seed: seed}) {
			out = append(out, p.RecipeID)
		}
		return out
	}
	if a, b := ids(5), ids(5); fmt.Sprint(a) != fmt.Sprint(b) {
		t.Errorf("same seed gave different picks: %v vs %v", a, b)
	}
	if a, b := ids(5), ids(6); fmt.Sprint(a) == fmt.Sprint(b) {
		t.Errorf("different seeds gave identical picks: %v", a)
	}
}

func TestImpressionsFadeIgnoredPicks(t *testing.T) {
	m := Build(library(12, 8), 1)
	shown := map[int]int{}
	for day := range uint64(20) {
		fb := map[int]Feedback{}
		for id, n := range shown {
			fb[id] = Feedback{Impressions: n}
		}
		for _, p := range m.Recommend(fb, Options{Limit: 6, Seed: day}) {
			shown[p.RecipeID]++
		}
	}
	most := 0
	for _, n := range shown {
		most = max(most, n)
	}
	if most > 6 {
		t.Errorf("a recipe was shown %d of 20 days despite being ignored", most)
	}
	if len(shown) < 40 {
		t.Errorf("only %d different recipes shown over 20 days", len(shown))
	}
}

func TestAppealFollowsUserScale(t *testing.T) {
	tests := []struct {
		rating  float64
		minutes int
		want    float64
	}{
		{10, 30, 1}, {7, 30, 0.5}, {6, 30, 1.0 / 3}, {6, 12, 0.5}, {4, 30, 0}, {1, 30, 0},
	}
	for _, tt := range tests {
		if got := appeal(tt.rating, tt.minutes); got < tt.want-1e-9 || got > tt.want+1e-9 {
			t.Errorf("appeal(%v, %d min) = %v, want %v", tt.rating, tt.minutes, got, tt.want)
		}
	}
}

// withEmbeddings gives each recipe a group direction plus noise, optionally
// pushed towards a shared direction so every pair looks similar — the
// crowding an instruction-prompted embedding model produces.
func withEmbeddings(recipes []Recipe, perGroup int, crowd float64) []Recipe {
	rng := rand.New(rand.NewPCG(3, 4))
	const dims = 32
	shared := make([]float64, dims)
	for d := range shared {
		shared[d] = rng.NormFloat64()
	}
	groups := map[int][]float64{}
	out := make([]Recipe, len(recipes))
	for i, r := range recipes {
		g := i / perGroup
		if groups[g] == nil {
			groups[g] = make([]float64, dims)
			for d := range dims {
				groups[g][d] = rng.NormFloat64()
			}
		}
		v := make([]float32, dims)
		for d := range dims {
			v[d] = float32(groups[g][d] + 0.3*rng.NormFloat64() + crowd*shared[d])
		}
		r.Embedding = v
		out[i] = r
	}
	return out
}

func TestCalibrationSurvivesCrowdedEmbeddings(t *testing.T) {
	base := library(12, 8)
	plain := Build(withEmbeddings(base, 8, 0), 1)
	crowded := Build(withEmbeddings(base, 8, 6), 1)
	if crowded.sameDish == sameDishFallback {
		t.Fatal("crowded model fell back to the fixed threshold instead of calibrating")
	}
	fb := map[int]Feedback{1: {CookCount: 1, AvgRating: rating(9)}}
	a := plain.Recommend(fb, Options{Limit: 10, Seed: 5, SimilarShare: 0.25})
	b := crowded.Recommend(fb, Options{Limit: 10, Seed: 5, SimilarShare: 0.25})
	if len(b) < 10 {
		t.Fatalf("crowded embeddings blocked picks as near-twins: got %d of 10", len(b))
	}
	same := 0
	for i := range a {
		if a[i].RecipeID == b[i].RecipeID {
			same++
		}
	}
	if same < 7 {
		t.Errorf("crowding the embeddings changed %d of 10 picks", 10-same)
	}
}
