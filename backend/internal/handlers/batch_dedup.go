package handlers

import (
	"sync"

	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/models"
)

// batchTracker stops concurrent batch slots from returning the same dish.
// Slots generate in parallel without seeing each other, and neighbouring
// cuisines share dishes (laksa is Malaysian and Singaporean), so each
// finished recipe is checked against the ones already accepted.
type batchTracker struct {
	mu       sync.Mutex
	accepted []database.RecipeFingerprint
}

// claim accepts recipe unless it duplicates one accepted earlier, in which
// case it returns that recipe's title. force accepts it regardless, for a
// slot that has run out of retries.
func (b *batchTracker) claim(recipe models.Recipe, force bool) (duplicateOf string, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if dups := findNearDuplicates(recipe, b.accepted); len(dups) > 0 && !force {
		return dups[0].Title, false
	}
	names := make([]string, len(recipe.Ingredients))
	for i, ing := range recipe.Ingredients {
		names[i] = ing.Name
	}
	b.accepted = append(b.accepted, database.RecipeFingerprint{
		Title:       recipe.Title,
		CuisineType: recipe.CuisineType,
		Ingredients: names,
	})
	return "", true
}
