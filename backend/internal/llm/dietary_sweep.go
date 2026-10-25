package llm

import (
	"context"
	"log/slog"

	"github.com/rubenwo/mise/internal/database"
)

// SweepDietaryLabels removes contradicted dietary labels across the library
// (recipes saved before the check existed, or entered by hand). Recipes
// without a conflict are not written. Safe to run on every startup.
func SweepDietaryLabels(ctx context.Context, q *database.Queries) {
	recipes, err := q.ListRecipesForDietaryCheck(ctx)
	if err != nil {
		slog.Warn("dietary label sweep: listing recipes failed", "err", err)
		return
	}
	fixed := 0
	for i := range recipes {
		if len(DropContradictedDiets(&recipes[i])) == 0 {
			continue
		}
		if err := q.SetRecipeDietary(ctx, recipes[i].ID, recipes[i].DietaryRestrictions); err != nil {
			slog.Warn("dietary label sweep: update failed", "recipe", recipes[i].ID, "err", err)
			continue
		}
		fixed++
	}
	if fixed > 0 {
		slog.Info("dietary label sweep: removed contradicted labels", "recipes", fixed)
	}
}
