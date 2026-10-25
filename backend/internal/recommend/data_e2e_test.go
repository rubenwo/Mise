//go:build e2e

// Runs the recommender over exported library data and prints a week of
// picks for manual review. Export with:
//
//	curl $MISE/api/recipes?limit=1000 > recipes.json
//	curl $MISE/api/recipes/eat-counts > eat.json
//	# emb.json: {"<recipe id>": [floats...]} from the embedding model
//	MISE_REC_DATA=<dir> go test -tags e2e -v -run Data ./internal/recommend/
package recommend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestDataWeekOfPicks(t *testing.T) {
	dir := os.Getenv("MISE_REC_DATA")
	if dir == "" {
		t.Skip("MISE_REC_DATA not set")
	}
	read := func(name string, v any) {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, v); err != nil {
			t.Fatal(err)
		}
	}
	var lib struct {
		Recipes []struct {
			ID          int      `json:"id"`
			Title       string   `json:"title"`
			Cuisine     string   `json:"cuisine_type"`
			Tags        []string `json:"tags"`
			Prep        int      `json:"prep_time_minutes"`
			Cook        int      `json:"cook_time_minutes"`
			Ingredients []struct {
				Name string `json:"name"`
			} `json:"ingredients"`
		} `json:"recipes"`
	}
	var eat []struct {
		RecipeID  int      `json:"recipe_id"`
		Count     int      `json:"count"`
		AvgRating *float64 `json:"avg_rating"`
	}
	emb := map[string][]float32{}
	read("recipes.json", &lib)
	read("eat.json", &eat)
	read("emb.json", &emb)

	var recipes []Recipe
	titles := map[int]string{}
	for _, r := range lib.Recipes {
		var ings []string
		for _, i := range r.Ingredients {
			ings = append(ings, i.Name)
		}
		recipes = append(recipes, Recipe{ID: r.ID, Title: r.Title, Cuisine: r.Cuisine, Tags: r.Tags,
			Ingredients: ings, Minutes: r.Prep + r.Cook, Embedding: emb[strconv.Itoa(r.ID)]})
		titles[r.ID] = r.Title
	}
	fb := map[int]Feedback{}
	for _, e := range eat {
		fb[e.RecipeID] = Feedback{CookCount: e.Count, AvgRating: e.AvgRating}
	}

	start := time.Now()
	m := Build(recipes, 1)
	t.Logf("built %d families over %d recipes in %v", m.Families(), len(recipes), time.Since(start))
	for f := range m.Families() {
		t.Logf("  family %2d: %s", f, m.FamilyLabel(f))
	}

	shown := map[int]int{}
	for day := range uint64(7) {
		for id, n := range shown {
			f := fb[id]
			f.Impressions = n
			fb[id] = f
		}
		picks := m.Recommend(fb, Options{Limit: 12, Seed: 20261026 + day, SimilarShare: 0.25})
		if day < 2 {
			t.Logf("day %d:", day+1)
			for _, p := range picks {
				t.Log(fmt.Sprintf("  %-7s %-60.60s %s", p.Lane, titles[p.RecipeID], p.Reason))
			}
		}
		for _, p := range picks {
			shown[p.RecipeID]++
		}
	}
	t.Logf("7 days: %d different recipes in %d slots", len(shown), 7*12)
}
