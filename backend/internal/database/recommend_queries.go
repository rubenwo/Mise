package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// RecommendRecipe is a recipe as the recommender and embedder need it.
type RecommendRecipe struct {
	ID          int
	Title       string
	Description string
	Cuisine     string
	Tags        []string
	Ingredients []string
	Minutes     int
	// Embedding fields are empty when the recipe has not been embedded.
	EmbeddingModel string
	TextHash       string
	Embedding      []float32
}

// ListRecommendRecipes returns every library recipe with its stored embedding, if any.
func (q *Queries) ListRecommendRecipes(ctx context.Context) ([]RecommendRecipe, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT r.id, r.title, r.description, r.cuisine_type, r.tags, r.ingredients,
			r.prep_time_minutes + r.cook_time_minutes,
			COALESCE(e.model, ''), COALESCE(e.text_hash, ''), e.embedding
		FROM recipes r
		LEFT JOIN recipe_embeddings e ON e.recipe_id = r.id
		ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RecommendRecipe
	for rows.Next() {
		var r RecommendRecipe
		var ingredientsJSON []byte
		if err := rows.Scan(&r.ID, &r.Title, &r.Description, &r.Cuisine, &r.Tags, &ingredientsJSON,
			&r.Minutes, &r.EmbeddingModel, &r.TextHash, &r.Embedding); err != nil {
			return nil, err
		}
		var ings []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(ingredientsJSON, &ings); err == nil {
			for _, ing := range ings {
				if ing.Name != "" {
					r.Ingredients = append(r.Ingredients, ing.Name)
				}
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertRecipeEmbedding stores a recipe's embedding.
func (q *Queries) UpsertRecipeEmbedding(ctx context.Context, recipeID int, model, textHash string, vec []float32) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO recipe_embeddings (recipe_id, model, text_hash, embedding, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (recipe_id) DO UPDATE
		SET model = EXCLUDED.model, text_hash = EXCLUDED.text_hash,
			embedding = EXCLUDED.embedding, updated_at = NOW()`,
		recipeID, model, textHash, vec)
	return err
}

// RecommendStateKey changes whenever the library or its embeddings change,
// so a cached recommender model knows when to rebuild.
func (q *Queries) RecommendStateKey(ctx context.Context) (string, error) {
	var recipes, embeddings int
	var recipesAt, embeddingsAt *time.Time
	err := q.pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM recipes), (SELECT MAX(updated_at) FROM recipes),
			(SELECT COUNT(*) FROM recipe_embeddings), (SELECT MAX(updated_at) FROM recipe_embeddings)`,
	).Scan(&recipes, &recipesAt, &embeddings, &embeddingsAt)
	if err != nil {
		return "", err
	}
	stamp := func(t *time.Time) int64 {
		if t == nil {
			return 0
		}
		return t.UnixNano()
	}
	return fmt.Sprintf("%d/%d/%d/%d", recipes, stamp(recipesAt), embeddings, stamp(embeddingsAt)), nil
}

// ListSkipCounts returns how often each recipe was planned and then marked
// "not making it".
func (q *Queries) ListSkipCounts(ctx context.Context) (map[int]int, error) {
	return q.countByRecipe(ctx, `
		SELECT recipe_id, COUNT(*) FROM meal_plan_recipes
		WHERE skipped = TRUE GROUP BY recipe_id`)
}

// ListImpressionCounts returns, per recipe that has never been put in a
// plan, the number of days it was recommended.
func (q *Queries) ListImpressionCounts(ctx context.Context) (map[int]int, error) {
	return q.countByRecipe(ctx, `
		SELECT ri.recipe_id, COUNT(*) FROM recommendation_impressions ri
		WHERE NOT EXISTS (SELECT 1 FROM meal_plan_recipes mpr WHERE mpr.recipe_id = ri.recipe_id)
		GROUP BY ri.recipe_id`)
}

func (q *Queries) countByRecipe(ctx context.Context, sql string) (map[int]int, error) {
	rows, err := q.pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var id, n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// RecordImpressions notes that these recipes were recommended today.
func (q *Queries) RecordImpressions(ctx context.Context, recipeIDs []int) error {
	if len(recipeIDs) == 0 {
		return nil
	}
	_, err := q.pool.Exec(ctx, `
		INSERT INTO recommendation_impressions (recipe_id)
		SELECT id FROM recipes WHERE id = ANY($1)
		ON CONFLICT DO NOTHING`, recipeIDs)
	return err
}

// RecipeFilterMeta is what library filters need to know about a recipe.
type RecipeFilterMeta struct {
	ID      int
	Cuisine string
	Minutes int
	Dietary []string
}

// ListRecipeFilterMeta returns cuisine, total time and dietary labels for
// every recipe, for filtering recommendations without loading full recipes.
func (q *Queries) ListRecipeFilterMeta(ctx context.Context) ([]RecipeFilterMeta, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT id, cuisine_type, prep_time_minutes + cook_time_minutes, dietary_restrictions
		FROM recipes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecipeFilterMeta
	for rows.Next() {
		var m RecipeFilterMeta
		if err := rows.Scan(&m.ID, &m.Cuisine, &m.Minutes, &m.Dietary); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListFavouriteRecipeIDs returns recipes whose average rating is at least
// minRating, best first (ties broken by how often they were cooked).
func (q *Queries) ListFavouriteRecipeIDs(ctx context.Context, minRating float64) ([]int, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT recipe_id FROM meal_plan_recipes
		WHERE completed = TRUE AND rating IS NOT NULL
		GROUP BY recipe_id
		HAVING AVG(rating) >= $1
		ORDER BY AVG(rating) DESC, COUNT(*) DESC, recipe_id`, minRating)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
