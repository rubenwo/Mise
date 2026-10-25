package ingredients

import (
	"math"
	"reflect"
	"testing"

	"github.com/rubenwo/mise/internal/models"
)

func TestNormalizeIngredientName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Butter (unsalted)", "butter"},
		{"butter, softened", "butter"},
		{"Freshly ground black pepper", "pepper"},
		{"Eggs", "egg"},
		{"molasses", "molasses"},
		{"Hummus", "hummus"},
		{"asparagus", "asparagus"},
		{"Garlic cloves", "garlic"},
		{"Extra virgin olive oil", "olive oil"},
		{"salt to taste", "salt"},
		{"Whole wheat flour", "flour"},
		{"large", "large"},
		{"  Courgettes ", "zucchini"},
		{"Basil leaves", "basil leaf"},
		{"cherry tomatoes", "cherry tomato"},
		{"Potatoes", "potato"},
		{"blueberries", "blueberry"},
		{"olives", "olive"},
		{"boneless, skinless chicken thighs", "chicken thigh"},
		{"chicken thighs, boneless", "chicken thigh"},
		{"coriander leaves", "cilantro"},
	}
	for _, tt := range tests {
		if got := Normalize(tt.in); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUnitToBase(t *testing.T) {
	tests := []struct {
		amount   float64
		unit     string
		wantAmt  float64
		wantUnit string
	}{
		{2, "kg", 2000, "g"},
		{1, "Cups", 240, "ml"},
		{1, " Tbsp ", 14.787, "ml"},
		{3, "pinch", 3, "pinch"},
		{1, "lb", 453.592, "g"},
	}
	for _, tt := range tests {
		amt, unit := unitToBase(tt.amount, tt.unit)
		if math.Abs(amt-tt.wantAmt) > 1e-9 || unit != tt.wantUnit {
			t.Errorf("unitToBase(%v, %q) = (%v, %q), want (%v, %q)", tt.amount, tt.unit, amt, unit, tt.wantAmt, tt.wantUnit)
		}
	}
}

func TestBaseToDisplay(t *testing.T) {
	tests := []struct {
		amount   float64
		unit     string
		wantAmt  float64
		wantUnit string
	}{
		{1500, "g", 1.5, "kg"},
		{500, "ml", 500, "ml"},
		{2000, "ml", 2, "l"},
		{3, "pinch", 3, "pinch"},
	}
	for _, tt := range tests {
		amt, unit := baseToDisplay(tt.amount, tt.unit)
		if amt != tt.wantAmt || unit != tt.wantUnit {
			t.Errorf("baseToDisplay(%v, %q) = (%v, %q), want (%v, %q)", tt.amount, tt.unit, amt, unit, tt.wantAmt, tt.wantUnit)
		}
	}
}

func TestRoundAmount(t *testing.T) {
	if got := roundAmount(12.4); got != 12 {
		t.Errorf("roundAmount(12.4) = %v, want 12", got)
	}
	if got := roundAmount(1.234); got != 1.23 {
		t.Errorf("roundAmount(1.234) = %v, want 1.23", got)
	}
}

func TestIngredientNameSimilarity(t *testing.T) {
	if got := nameSimilarity("chicken breast", "chickenbreast"); got != 1.0 {
		t.Errorf("compact-equal names: got %v, want 1.0", got)
	}
	if got := nameSimilarity("tomato", "chocolate"); got >= 0.85 {
		t.Errorf("unrelated names scored %v, want < 0.85", got)
	}
}

func TestFuzzyCanonicalNames(t *testing.T) {
	got := canonicalNames([]string{"chickenbreast", "chicken breast", "tomato"})
	want := map[string]string{
		"chickenbreast":  "chicken breast",
		"chicken breast": "chicken breast",
		"tomato":         "tomato",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("canonicalNames = %v, want %v", got, want)
	}
}

func TestConsolidateIngredients(t *testing.T) {
	in := []models.AggregatedIngredient{
		{Name: "Flour", Amount: 200, Unit: "g", Recipes: []string{"A"}},
		{Name: "all-purpose flour", Amount: 1, Unit: "cups", Recipes: []string{"B"}},
		{Name: "Eggs", Amount: 2, Unit: "", Recipes: []string{"A"}},
		{Name: "egg", Amount: 1, Unit: "", Recipes: []string{"B"}},
		{Name: "Milk", Amount: 0.5, Unit: "l", Recipes: []string{"A"}},
		{Name: "Onion", Amount: 1.5, Unit: "", Recipes: []string{"B"}},
	}
	want := []models.AggregatedIngredient{
		{Name: "Egg", Amount: 3, Unit: "", Recipes: []string{"A", "B"}},
		// 200 g + 240 ml * 0.53 g/ml = 327.2 g
		{Name: "Flour", Amount: 327, Unit: "g", Recipes: []string{"A", "B"}},
		{Name: "Milk", Amount: 500, Unit: "ml", Recipes: []string{"A"}},
		// countable units round up
		{Name: "Onion", Amount: 2, Unit: "", Recipes: []string{"B"}},
	}
	got := Consolidate(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Consolidate =\n  %+v\nwant\n  %+v", got, want)
	}
}

func TestConsolidateIngredients_UnknownDensityStaysSplit(t *testing.T) {
	in := []models.AggregatedIngredient{
		{Name: "Cheese", Amount: 100, Unit: "g", Recipes: []string{"A"}},
		{Name: "cheese", Amount: 1, Unit: "cup", Recipes: []string{"B"}},
	}
	got := Consolidate(in)
	if len(got) != 2 {
		t.Fatalf("expected cheese to stay split across g/ml, got %+v", got)
	}

	dupes := FindRemainingDuplicates(got)
	if len(dupes) != 1 || len(dupes[0]) != 2 {
		t.Fatalf("FindRemainingDuplicates = %+v, want one group of two", dupes)
	}

	merged := []models.AggregatedIngredient{{Name: "Cheese", Amount: 340, Unit: "g", Recipes: []string{"A", "B"}}}
	withOther := append(got, models.AggregatedIngredient{Name: "Apple", Amount: 1, Unit: ""})
	final := ApplyMerged(withOther, dupes, merged)
	want := []models.AggregatedIngredient{
		{Name: "Apple", Amount: 1, Unit: ""},
		{Name: "Cheese", Amount: 340, Unit: "g", Recipes: []string{"A", "B"}},
	}
	if !reflect.DeepEqual(final, want) {
		t.Errorf("ApplyMerged =\n  %+v\nwant\n  %+v", final, want)
	}
}

func TestAggregate_ScalesToPlannedServings(t *testing.T) {
	plan := &models.MealPlan{Recipes: []models.MealPlanRecipe{
		{Servings: 4, Recipe: models.Recipe{Title: "Pancakes", Servings: 2, Ingredients: []models.Ingredient{
			{Name: "Milk", Amount: 250, Unit: "ml"},
			{Name: "Eggs", Amount: 1, Unit: ""},
		}}},
		{Servings: 2, Recipe: models.Recipe{Title: "Omelette", Servings: 2, Ingredients: []models.Ingredient{
			{Name: "egg", Amount: 3, Unit: ""},
		}}},
	}}
	want := []models.AggregatedIngredient{
		{Name: "Egg", Amount: 5, Unit: "", Recipes: []string{"Omelette", "Pancakes"}},
		{Name: "Milk", Amount: 500, Unit: "ml", Recipes: []string{"Pancakes"}},
	}
	if got := Aggregate(plan); !reflect.DeepEqual(got, want) {
		t.Errorf("Aggregate =\n  %+v\nwant\n  %+v", got, want)
	}
}
