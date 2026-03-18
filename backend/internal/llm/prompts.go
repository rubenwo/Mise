package llm

import (
	"fmt"
	"strings"

	"github.com/rubenwoldhuis/recipes/internal/models"
)

const systemPrompt = `You are a creative chef and recipe developer. Generate detailed, practical dinner recipes.

IMPORTANT: You MUST respond with valid JSON matching this exact structure:
{
  "title": "Recipe Title",
  "description": "A brief description of the dish",
  "cuisine_type": "Italian",
  "prep_time_minutes": 15,
  "cook_time_minutes": 30,
  "servings": 4,
  "difficulty": "easy",
  "ingredients": [
    {"name": "ingredient name", "amount": 1.5, "unit": "cups", "notes": "optional notes"}
  ],
  "instructions": [
    "Step 1: Do something",
    "Step 2: Do something else"
  ],
  "dietary_restrictions": ["vegetarian"],
  "tags": ["quick", "healthy"]
}

Rules:
- difficulty must be one of: easy, medium, hard
- All amounts must be numbers (not strings)
- Include at least 3 ingredients and 3 instructions
- Be specific with measurements and cooking times
- Use your tools to search for inspiration and avoid duplicating the user's existing recipes
- Respond ONLY with the JSON object, no other text`

func BuildGeneratePrompt(req models.GenerateRequest) string {
	var parts []string
	parts = append(parts, "Generate a dinner recipe")

	if req.CuisineType != "" {
		parts = append(parts, fmt.Sprintf("from %s cuisine", req.CuisineType))
	}
	if len(req.DietaryRestrictions) > 0 {
		parts = append(parts, fmt.Sprintf("that is %s", strings.Join(req.DietaryRestrictions, " and ")))
	}
	if req.MaxPrepTime > 0 {
		parts = append(parts, fmt.Sprintf("with prep time under %d minutes", req.MaxPrepTime))
	}
	if req.Difficulty != "" {
		parts = append(parts, fmt.Sprintf("at %s difficulty level", req.Difficulty))
	}
	if req.Servings > 0 {
		parts = append(parts, fmt.Sprintf("serving %d people", req.Servings))
	}
	if req.AdditionalNotes != "" {
		parts = append(parts, fmt.Sprintf("with these preferences: %s", req.AdditionalNotes))
	}

	return strings.Join(parts, " ") + "."
}

func BuildRefinePrompt(recipe models.Recipe, feedback string) string {
	return fmt.Sprintf(`Here is a recipe that needs refinement:

Title: %s
Description: %s
Cuisine: %s

The user wants the following changes: %s

Generate an improved version of this recipe incorporating the feedback. Respond with the complete updated recipe JSON.`, recipe.Title, recipe.Description, recipe.CuisineType, feedback)
}

func SystemPrompt() string {
	return systemPrompt
}
