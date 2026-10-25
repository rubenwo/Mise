package handlers

import (
	"testing"

	"github.com/rubenwo/mise/internal/models"
)

func recipeWith(title, cuisine string, ingredients ...string) models.Recipe {
	r := models.Recipe{Title: title, CuisineType: cuisine}
	for _, name := range ingredients {
		r.Ingredients = append(r.Ingredients, models.Ingredient{Name: name})
	}
	return r
}

func TestBatchTracker_RejectsSameDishAcrossCuisines(t *testing.T) {
	var b batchTracker
	laksa := []string{"rice noodles", "coconut milk", "laksa paste", "chicken thigh", "bean sprouts", "tofu puffs"}

	if _, ok := b.claim(recipeWith("Chicken Laksa", "Malaysian", laksa...), false); !ok {
		t.Fatal("first recipe should be accepted")
	}
	if _, ok := b.claim(recipeWith("Beef Stew", "Irish", "beef chuck", "potato", "carrot", "stout"), false); !ok {
		t.Fatal("a different dish should be accepted")
	}
	dup, ok := b.claim(recipeWith("Chicken Laksa", "Singaporean", laksa...), false)
	if ok || dup != "Chicken Laksa" {
		t.Fatalf("second laksa: ok=%v dup=%q, want rejected as duplicate of Chicken Laksa", ok, dup)
	}
	if _, ok := b.claim(recipeWith("Chicken Laksa", "Singaporean", laksa...), true); !ok {
		t.Fatal("force should accept a duplicate once retries are used up")
	}
}
