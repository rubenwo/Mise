package handlers

import (
	"slices"
	"testing"

	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/models"
)

func TestWeightedSample(t *testing.T) {
	pool := []database.RecipeSummary{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}
	uniform := func(database.RecipeSummary) float64 { return 1 }

	if got := weightedSample(pool, 0, uniform); got != nil {
		t.Errorf("n=0: got %v, want nil", got)
	}
	if got := weightedSample(nil, 3, uniform); got != nil {
		t.Errorf("empty pool: got %v, want nil", got)
	}

	all := weightedSample(pool, 10, uniform)
	if len(all) != len(pool) {
		t.Fatalf("n>len: got %d items, want %d", len(all), len(pool))
	}

	got := weightedSample(pool, 2, uniform)
	if len(got) != 2 || got[0].ID == got[1].ID {
		t.Errorf("n=2: got %v, want two distinct items", got)
	}
}

func TestWeightedSample_FavoursHeavyItems(t *testing.T) {
	pool := []database.RecipeSummary{{ID: 1}, {ID: 2}}
	weight := func(s database.RecipeSummary) float64 {
		if s.ID == 1 {
			return 100
		}
		return 0.01
	}
	heavy := 0
	for range 200 {
		if weightedSample(pool, 1, weight)[0].ID == 1 {
			heavy++
		}
	}
	if heavy < 190 {
		t.Errorf("heavy item picked %d/200 times, want >= 190", heavy)
	}
}

func TestDiversifyCuisines(t *testing.T) {
	selected := []database.RecipeSummary{{ID: 1, CuisineType: "Italian"}, {ID: 2, CuisineType: "Italian"}}
	pool := []database.RecipeSummary{
		{ID: 1, CuisineType: "Italian"},
		{ID: 2, CuisineType: "Italian"},
		{ID: 3, CuisineType: "Thai"},
	}
	got := diversifyCuisines(selected, pool, func(database.RecipeSummary) float64 { return 1 })
	ids := []int{got[0].ID, got[1].ID}
	if !slices.Equal(ids, []int{3, 2}) {
		t.Errorf("diversifyCuisines ids = %v, want [3 2]", ids)
	}
}

func TestRecipeSimilarity(t *testing.T) {
	a := database.RecipeDedup{Title: "Spaghetti Carbonara", Description: "Classic Roman pasta with eggs and pecorino"}
	b := database.RecipeDedup{Title: "Spaghetti alla Carbonara", Description: "Roman pasta with eggs, guanciale and pecorino"}
	c := database.RecipeDedup{Title: "Thai Green Curry", Description: "Coconut curry with chicken and basil"}

	if got := recipeSimilarity(a, b); got < 0.60 {
		t.Errorf("carbonara variants scored %v, want >= 0.60", got)
	}
	if got := recipeSimilarity(a, c); got >= 0.60 {
		t.Errorf("unrelated recipes scored %v, want < 0.60", got)
	}
}

func TestFindNearDuplicates(t *testing.T) {
	recipe := models.Recipe{
		Title:       "Chicken Tikka Masala",
		CuisineType: "Indian",
		Ingredients: []models.Ingredient{{Name: "chicken thighs"}, {Name: "yogurt"}, {Name: "tomato puree"}, {Name: "garam masala"}},
	}
	fps := []database.RecipeFingerprint{
		{ID: 1, Title: "Tikka Masala Chicken", CuisineType: "Indian", Ingredients: []string{"chicken thigh", "plain yoghurt", "tomato paste", "garam masala"}},
		{ID: 2, Title: "Pad Thai", CuisineType: "Thai", Ingredients: []string{"rice noodles", "shrimp", "peanuts", "tamarind"}},
	}
	got := findNearDuplicates(recipe, fps)
	if len(got) != 1 || got[0].ID != 1 {
		t.Errorf("findNearDuplicates = %+v, want only ID 1", got)
	}
}
