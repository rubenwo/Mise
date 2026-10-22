package handlers

import (
	"strings"

	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/ingredients"
	"github.com/rubenwo/mise/internal/models"
	"github.com/rubenwo/mise/internal/textsim"
)

// recipeSimilarity returns a score in [0,1] combining title and description
// similarity. Title is weighted more heavily (80 %).
func recipeSimilarity(a, b database.RecipeDedup) float64 {
	t1 := textsim.Normalize(a.Title)
	t2 := textsim.Normalize(b.Title)
	d1 := textsim.Normalize(a.Description)
	d2 := textsim.Normalize(b.Description)

	titleScore := max(textsim.JaccardWords(t1, t2), textsim.JaccardBigrams(t1, t2))
	descScore := textsim.JaccardWords(d1, d2)
	return 0.80*titleScore + 0.20*descScore
}

// nearDuplicateThreshold is the minimum concept-similarity score at which a
// generated recipe is considered a near-duplicate of an existing one.
// Combines title Jaccard (30%), ingredient-set Jaccard (60%), and a cuisine
// bonus (10%). Empirically: same dish different name ≈ 0.66, clearly distinct
// dishes ≈ 0.20.
const nearDuplicateThreshold = 0.60

// ingredientNameSet returns the set of normalized names for a slice of raw names.
func ingredientNameSet(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		if norm := ingredients.Normalize(n); norm != "" {
			set[norm] = struct{}{}
		}
	}
	return set
}

// ingredientJaccard returns the Jaccard similarity of two ingredient name sets.
func ingredientJaccard(a, b []string) float64 {
	sa, sb := ingredientNameSet(a), ingredientNameSet(b)
	if len(sa) == 0 && len(sb) == 0 {
		return 0
	}
	intersection := 0
	for k := range sa {
		if _, ok := sb[k]; ok {
			intersection++
		}
	}
	union := len(sa) + len(sb) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// recipeConceptSimilarity returns a [0,1] score measuring how semantically
// similar a newly generated recipe is to an existing fingerprint.
func recipeConceptSimilarity(r models.Recipe, fp database.RecipeFingerprint) float64 {
	t1 := textsim.Normalize(r.Title)
	t2 := textsim.Normalize(fp.Title)
	titleScore := max(textsim.JaccardWords(t1, t2), textsim.JaccardBigrams(t1, t2))

	newNames := make([]string, len(r.Ingredients))
	for i, ing := range r.Ingredients {
		newNames[i] = ing.Name
	}
	ingScore := ingredientJaccard(newNames, fp.Ingredients)

	cuisineBonus := 0.0
	if fp.CuisineType != "" && strings.EqualFold(r.CuisineType, fp.CuisineType) {
		cuisineBonus = 0.10
	}

	score := 0.30*titleScore + 0.60*ingScore + cuisineBonus
	if score > 1.0 {
		return 1.0
	}
	return score
}

// findNearDuplicates returns existing fingerprints that are conceptually similar
// to the given recipe, capped at 3 results to avoid UI noise.
func findNearDuplicates(r models.Recipe, fingerprints []database.RecipeFingerprint) []database.RecipeFingerprint {
	var matches []database.RecipeFingerprint
	for _, fp := range fingerprints {
		if recipeConceptSimilarity(r, fp) >= nearDuplicateThreshold {
			matches = append(matches, fp)
			if len(matches) == 3 {
				break
			}
		}
	}
	return matches
}
