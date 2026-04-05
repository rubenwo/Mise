package models

import "time"

type MealPlan struct {
	ID        int              `json:"id"`
	Name      string           `json:"name"`
	Status    string           `json:"status"`
	Recipes   []MealPlanRecipe `json:"recipes,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

type MealPlanRecipe struct {
	ID        int    `json:"id"`
	RecipeID  int    `json:"recipe_id"`
	Servings  int    `json:"servings"`
	SortOrder int    `json:"sort_order"`
	Completed bool   `json:"completed"`
	Recipe    Recipe `json:"recipe"`
}

type AddPlanRecipeRequest struct {
	RecipeID int `json:"recipe_id"`
	Servings int `json:"servings"`
}

type UpdatePlanRecipeRequest struct {
	Servings  *int  `json:"servings,omitempty"`
	Completed *bool `json:"completed,omitempty"`
}

type UpdateMealPlanRequest struct {
	Status *string `json:"status,omitempty"`
}

type RandomizeRequest struct {
	Servings    []int `json:"servings"`
	ExcludedIDs []int `json:"excluded_ids,omitempty"`
}

type AggregatedIngredient struct {
	Name    string   `json:"name"`
	Amount  float64  `json:"amount"`
	Unit    string   `json:"unit"`
	Recipes []string `json:"recipes"`
}
