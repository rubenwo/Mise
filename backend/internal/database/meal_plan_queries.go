package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rubenwo/mise/internal/models"
)

func (q *Queries) CreateMealPlan(ctx context.Context, name string) (*models.MealPlan, error) {
	p := &models.MealPlan{Name: name, Status: "draft"}
	err := q.pool.QueryRow(ctx, `
		INSERT INTO meal_plans (name) VALUES ($1)
		RETURNING id, created_at, updated_at`, name,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (q *Queries) ListMealPlans(ctx context.Context) ([]models.MealPlan, error) {
	// Summary columns let the plans list show progress and thumbnails without
	// loading every plan's recipes.
	rows, err := q.pool.Query(ctx, `
		SELECT mp.id, mp.name, mp.status, mp.created_at, mp.updated_at,
			COUNT(mpr.id),
			COUNT(CASE WHEN mpr.completed THEN 1 END)
		FROM meal_plans mp
		LEFT JOIN meal_plan_recipes mpr ON mpr.meal_plan_id = mp.id
		GROUP BY mp.id, mp.name, mp.status, mp.created_at, mp.updated_at
		ORDER BY mp.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []models.MealPlan
	index := map[int]int{}
	for rows.Next() {
		var p models.MealPlan
		if err := rows.Scan(&p.ID, &p.Name, &p.Status, &p.CreatedAt, &p.UpdatedAt,
			&p.RecipeCount, &p.CookedCount); err != nil {
			return nil, err
		}
		index[p.ID] = len(plans)
		plans = append(plans, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	imgRows, err := q.pool.Query(ctx, `
		SELECT mpr.meal_plan_id, r.image_url
		FROM meal_plan_recipes mpr
		JOIN recipes r ON r.id = mpr.recipe_id
		WHERE r.image_url IS NOT NULL AND r.image_url <> ''
		ORDER BY mpr.meal_plan_id, mpr.sort_order, mpr.id`)
	if err != nil {
		return nil, err
	}
	defer imgRows.Close()
	for imgRows.Next() {
		var planID int
		var url string
		if err := imgRows.Scan(&planID, &url); err != nil {
			return nil, err
		}
		if i, ok := index[planID]; ok && len(plans[i].PreviewImages) < 4 {
			plans[i].PreviewImages = append(plans[i].PreviewImages, url)
		}
	}
	return plans, imgRows.Err()
}

func (q *Queries) GetMealPlan(ctx context.Context, id int) (*models.MealPlan, error) {
	p := &models.MealPlan{}
	err := q.pool.QueryRow(ctx, `
		SELECT id, name, status, created_at, updated_at
		FROM meal_plans WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}

	rows, err := q.pool.Query(ctx, `
		SELECT mpr.id, mpr.recipe_id, mpr.servings, mpr.sort_order, mpr.completed, mpr.skipped,
			mpr.completed_at, mpr.rating,
			r.id, r.title, r.description, r.cuisine_type, r.prep_time_minutes, r.cook_time_minutes,
			r.servings, r.difficulty, r.ingredients, r.instructions, r.dietary_restrictions, r.tags,
			r.generated_by_model, r.generation_prompt, COALESCE(r.image_url, ''), r.created_at, r.updated_at
		FROM meal_plan_recipes mpr
		JOIN recipes r ON r.id = mpr.recipe_id
		WHERE mpr.meal_plan_id = $1
		ORDER BY mpr.sort_order, mpr.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var mpr models.MealPlanRecipe
		var ingredientsJSON, instructionsJSON []byte
		var ratingNullable *int16
		if err := rows.Scan(
			&mpr.ID, &mpr.RecipeID, &mpr.Servings, &mpr.SortOrder, &mpr.Completed, &mpr.Skipped,
			&mpr.CompletedAt, &ratingNullable,
			&mpr.Recipe.ID, &mpr.Recipe.Title, &mpr.Recipe.Description, &mpr.Recipe.CuisineType,
			&mpr.Recipe.PrepTimeMinutes, &mpr.Recipe.CookTimeMinutes, &mpr.Recipe.Servings,
			&mpr.Recipe.Difficulty, &ingredientsJSON, &instructionsJSON,
			&mpr.Recipe.DietaryRestrictions, &mpr.Recipe.Tags,
			&mpr.Recipe.GeneratedByModel, &mpr.Recipe.GenerationPrompt,
			&mpr.Recipe.ImageURL, &mpr.Recipe.CreatedAt, &mpr.Recipe.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if ratingNullable != nil {
			r := int(*ratingNullable)
			mpr.Rating = &r
		}
		if err := json.Unmarshal(ingredientsJSON, &mpr.Recipe.Ingredients); err != nil {
			return nil, fmt.Errorf("unmarshaling ingredients: %w", err)
		}
		if err := json.Unmarshal(instructionsJSON, &mpr.Recipe.Instructions); err != nil {
			return nil, fmt.Errorf("unmarshaling instructions: %w", err)
		}
		p.Recipes = append(p.Recipes, mpr)
	}
	return p, rows.Err()
}

func (q *Queries) DeleteMealPlan(ctx context.Context, id int) error {
	tag, err := q.pool.Exec(ctx, "DELETE FROM meal_plans WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (q *Queries) UpdateMealPlanStatus(ctx context.Context, id int, status string) error {
	tag, err := q.pool.Exec(ctx, `
		UPDATE meal_plans SET status = $2, updated_at = NOW() WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (q *Queries) AddRecipeToPlan(ctx context.Context, planID, recipeID, servings int) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO meal_plan_recipes (meal_plan_id, recipe_id, servings, sort_order)
		VALUES ($1, $2, $3, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM meal_plan_recipes WHERE meal_plan_id = $1))
		ON CONFLICT (meal_plan_id, recipe_id) DO NOTHING`,
		planID, recipeID, servings)
	return err
}

func (q *Queries) RemoveRecipeFromPlan(ctx context.Context, planID, recipeID int) error {
	tag, err := q.pool.Exec(ctx, `
		DELETE FROM meal_plan_recipes WHERE meal_plan_id = $1 AND recipe_id = $2`, planID, recipeID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

const maxIngredientSearchTerms = 20

func (q *Queries) SearchRecipesByIngredients(ctx context.Context, ingredientNames []string) ([]models.Recipe, error) {
	if len(ingredientNames) == 0 {
		return nil, nil
	}
	if len(ingredientNames) > maxIngredientSearchTerms {
		ingredientNames = ingredientNames[:maxIngredientSearchTerms]
	}
	// Build a query that searches for recipes containing any of the named ingredients
	// using JSONB array element text matching
	conditions := make([]string, len(ingredientNames))
	args := make([]any, len(ingredientNames))
	for i, name := range ingredientNames {
		conditions[i] = fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(ingredients) elem WHERE LOWER(elem->>'name') LIKE '%%' || LOWER($%d) || '%%')", i+1)
		args[i] = name
	}

	query := fmt.Sprintf(`
		SELECT id, title, description, cuisine_type, prep_time_minutes, cook_time_minutes,
			servings, difficulty, ingredients, instructions, dietary_restrictions, tags,
			generated_by_model, generation_prompt, created_at, updated_at
		FROM recipes WHERE %s ORDER BY created_at DESC LIMIT 10`,
		strings.Join(conditions, " OR "))

	rows, err := q.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recipes, _, err := scanRecipes(rows, 0)
	return recipes, err
}

func (q *Queries) ListEatenRecipeIDs(ctx context.Context) (map[int]bool, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT DISTINCT recipe_id FROM meal_plan_recipes mpr
		JOIN meal_plans mp ON mp.id = mpr.meal_plan_id
		WHERE mp.status IN ('active', 'completed')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	eaten := make(map[int]bool)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		eaten[id] = true
	}
	return eaten, rows.Err()
}

// SwapPlanRecipe replaces an uncooked dinner with another recipe in place,
// keeping its servings and position. It returns pgx.ErrNoRows when oldID is
// not an uncooked dinner of the plan.
func (q *Queries) SwapPlanRecipe(ctx context.Context, planID, oldID, newID int) error {
	tag, err := q.pool.Exec(ctx, `
		UPDATE meal_plan_recipes
		SET recipe_id = $3, skipped = FALSE, rating = NULL, completed_at = NULL
		WHERE meal_plan_id = $1 AND recipe_id = $2 AND completed = FALSE`,
		planID, oldID, newID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (q *Queries) ReplacePlanRecipes(ctx context.Context, planID int, recipeIDs []int, servings []int) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "DELETE FROM meal_plan_recipes WHERE meal_plan_id = $1", planID); err != nil {
		return err
	}

	for i, recipeID := range recipeIDs {
		s := 4
		if i < len(servings) {
			s = servings[i]
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO meal_plan_recipes (meal_plan_id, recipe_id, servings, sort_order)
			VALUES ($1, $2, $3, $4)`, planID, recipeID, s, i+1); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (q *Queries) GetPlanNormalizedIngredients(ctx context.Context, planID int) ([]byte, error) {
	var data []byte
	err := q.pool.QueryRow(ctx,
		"SELECT normalized_ingredients FROM meal_plans WHERE id = $1", planID,
	).Scan(&data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (q *Queries) SetPlanNormalizedIngredients(ctx context.Context, planID int, data []byte) error {
	_, err := q.pool.Exec(ctx,
		"UPDATE meal_plans SET normalized_ingredients = $2 WHERE id = $1", planID, data)
	return err
}

// ListGroceryChecks returns the names of grocery items ticked off for a plan.
func (q *Queries) ListGroceryChecks(ctx context.Context, planID int) ([]string, error) {
	rows, err := q.pool.Query(ctx,
		"SELECT name FROM plan_grocery_checks WHERE meal_plan_id = $1 ORDER BY name", planID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// SetGroceryCheck ticks a grocery item off (checked) or puts it back.
func (q *Queries) SetGroceryCheck(ctx context.Context, planID int, name string, checked bool) error {
	if checked {
		_, err := q.pool.Exec(ctx, `
			INSERT INTO plan_grocery_checks (meal_plan_id, name) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, planID, name)
		return err
	}
	_, err := q.pool.Exec(ctx,
		"DELETE FROM plan_grocery_checks WHERE meal_plan_id = $1 AND name = $2", planID, name)
	return err
}

// SavePlanAHSearch stores the latest Albert Heijn search result for a plan.
func (q *Queries) SavePlanAHSearch(ctx context.Context, planID int, result []byte) (time.Time, error) {
	var at time.Time
	err := q.pool.QueryRow(ctx, `
		INSERT INTO plan_ah_searches (meal_plan_id, result, searched_at) VALUES ($1, $2, NOW())
		ON CONFLICT (meal_plan_id) DO UPDATE SET result = EXCLUDED.result, searched_at = EXCLUDED.searched_at
		RETURNING searched_at`, planID, result).Scan(&at)
	return at, err
}

// GetPlanAHSearch returns the latest stored Albert Heijn search, or
// pgx.ErrNoRows if the plan was never searched.
func (q *Queries) GetPlanAHSearch(ctx context.Context, planID int) ([]byte, time.Time, error) {
	var data []byte
	var at time.Time
	err := q.pool.QueryRow(ctx,
		"SELECT result, searched_at FROM plan_ah_searches WHERE meal_plan_id = $1", planID,
	).Scan(&data, &at)
	return data, at, err
}

func (q *Queries) InvalidatePlanIngredients(ctx context.Context, planID int) error {
	_, err := q.pool.Exec(ctx,
		"UPDATE meal_plans SET normalized_ingredients = NULL WHERE id = $1", planID)
	return err
}

// UpdatePlanRecipe applies any subset of {servings, completed, skipped, rating} to one row.
// Toggling completed=true also stamps completed_at = NOW() and clears skipped.
// Toggling completed=false clears completed_at AND rating (rating-without-eaten makes no sense).
// Toggling skipped=true clears completed/completed_at/rating (mutually exclusive states).
// Rating semantics: nil = leave alone, 0 = clear, 1-10 = set.
func (q *Queries) UpdatePlanRecipe(ctx context.Context, planID, recipeID int, servings *int, completed *bool, skipped *bool, rating *int) error {
	if servings != nil {
		if _, err := q.pool.Exec(ctx, `
			UPDATE meal_plan_recipes SET servings = $3 WHERE meal_plan_id = $1 AND recipe_id = $2`,
			planID, recipeID, *servings); err != nil {
			return err
		}
	}
	if completed != nil {
		if *completed {
			if _, err := q.pool.Exec(ctx, `
				UPDATE meal_plan_recipes
				SET completed = TRUE, skipped = FALSE,
				    completed_at = COALESCE(completed_at, NOW())
				WHERE meal_plan_id = $1 AND recipe_id = $2`,
				planID, recipeID); err != nil {
				return err
			}
		} else {
			if _, err := q.pool.Exec(ctx, `
				UPDATE meal_plan_recipes
				SET completed = FALSE, completed_at = NULL, rating = NULL
				WHERE meal_plan_id = $1 AND recipe_id = $2`,
				planID, recipeID); err != nil {
				return err
			}
		}
	}
	if skipped != nil {
		if *skipped {
			if _, err := q.pool.Exec(ctx, `
				UPDATE meal_plan_recipes
				SET skipped = TRUE, completed = FALSE, completed_at = NULL, rating = NULL
				WHERE meal_plan_id = $1 AND recipe_id = $2`,
				planID, recipeID); err != nil {
				return err
			}
		} else {
			if _, err := q.pool.Exec(ctx, `
				UPDATE meal_plan_recipes SET skipped = FALSE
				WHERE meal_plan_id = $1 AND recipe_id = $2`,
				planID, recipeID); err != nil {
				return err
			}
		}
	}
	if rating != nil {
		var ratingValue any
		if *rating == 0 {
			ratingValue = nil
		} else {
			if *rating < 1 || *rating > 10 {
				return fmt.Errorf("rating must be 0 (clear) or 1-10, got %d", *rating)
			}
			ratingValue = *rating
		}
		if _, err := q.pool.Exec(ctx, `
			UPDATE meal_plan_recipes SET rating = $3 WHERE meal_plan_id = $1 AND recipe_id = $2`,
			planID, recipeID, ratingValue); err != nil {
			return err
		}
	}
	return nil
}

// GetRecipeHistory returns every plan a recipe has appeared on, newest cooked first.
// Plans where the recipe was never marked done sort to the bottom (NULLS LAST).
func (q *Queries) GetRecipeHistory(ctx context.Context, recipeID int) ([]models.RecipeHistoryEntry, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT mp.id, mp.name, mp.status, mpr.completed, mpr.completed_at, mpr.rating
		FROM meal_plan_recipes mpr
		JOIN meal_plans mp ON mp.id = mpr.meal_plan_id
		WHERE mpr.recipe_id = $1
		ORDER BY mpr.completed_at DESC NULLS LAST, mp.created_at DESC`, recipeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.RecipeHistoryEntry
	for rows.Next() {
		var e models.RecipeHistoryEntry
		var ratingNullable *int16
		if err := rows.Scan(&e.PlanID, &e.PlanName, &e.PlanStatus, &e.Completed, &e.CompletedAt, &ratingNullable); err != nil {
			return nil, err
		}
		if ratingNullable != nil {
			r := int(*ratingNullable)
			e.Rating = &r
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListRecipeEatCounts returns one stats row per recipe that has been completed
// at least once. Recipes that were never marked done are absent from the result.
func (q *Queries) ListRecipeEatCounts(ctx context.Context) ([]models.RecipeEatStats, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT recipe_id,
			COUNT(*) AS eat_count,
			MAX(completed_at) AS last_cooked_at,
			AVG(rating)::float8 AS avg_rating,
			COUNT(rating) AS rated_count
		FROM meal_plan_recipes
		WHERE completed = TRUE
		GROUP BY recipe_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.RecipeEatStats
	for rows.Next() {
		var s models.RecipeEatStats
		if err := rows.Scan(&s.RecipeID, &s.Count, &s.LastCookedAt, &s.AvgRating, &s.RatedCount); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
