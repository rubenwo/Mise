package handlers

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
	}
	for _, tt := range tests {
		if got := normalizeIngredientName(tt.in); got != tt.want {
			t.Errorf("normalizeIngredientName(%q) = %q, want %q", tt.in, got, tt.want)
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

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"kitten", "sitting", 3},
		{"", "abc", 3},
		{"abc", "", 3},
		{"same", "same", 0},
		{"café", "cafe", 1},
	}
	for _, tt := range tests {
		if got := levenshteinDistance(tt.a, tt.b); got != tt.want {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestIngredientNameSimilarity(t *testing.T) {
	if got := ingredientNameSimilarity("chicken breast", "chickenbreast"); got != 1.0 {
		t.Errorf("compact-equal names: got %v, want 1.0", got)
	}
	if got := ingredientNameSimilarity("tomato", "chocolate"); got >= 0.85 {
		t.Errorf("unrelated names scored %v, want < 0.85", got)
	}
}

func TestFuzzyCanonicalNames(t *testing.T) {
	got := fuzzyCanonicalNames([]string{"chickenbreast", "chicken breast", "tomato"})
	want := map[string]string{
		"chickenbreast":  "chicken breast",
		"chicken breast": "chicken breast",
		"tomato":         "tomato",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fuzzyCanonicalNames = %v, want %v", got, want)
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
	got := consolidateIngredients(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("consolidateIngredients =\n  %+v\nwant\n  %+v", got, want)
	}
}

func TestConsolidateIngredients_UnknownDensityStaysSplit(t *testing.T) {
	in := []models.AggregatedIngredient{
		{Name: "Cheese", Amount: 100, Unit: "g", Recipes: []string{"A"}},
		{Name: "cheese", Amount: 1, Unit: "cup", Recipes: []string{"B"}},
	}
	got := consolidateIngredients(in)
	if len(got) != 2 {
		t.Fatalf("expected cheese to stay split across g/ml, got %+v", got)
	}

	dupes := findRemainingDuplicates(got)
	if len(dupes) != 1 || len(dupes[0]) != 2 {
		t.Fatalf("findRemainingDuplicates = %+v, want one group of two", dupes)
	}

	merged := []models.AggregatedIngredient{{Name: "Cheese", Amount: 340, Unit: "g", Recipes: []string{"A", "B"}}}
	withOther := append(got, models.AggregatedIngredient{Name: "Apple", Amount: 1, Unit: ""})
	final := applyLLMDedup(withOther, dupes, merged)
	want := []models.AggregatedIngredient{
		{Name: "Apple", Amount: 1, Unit: ""},
		{Name: "Cheese", Amount: 340, Unit: "g", Recipes: []string{"A", "B"}},
	}
	if !reflect.DeepEqual(final, want) {
		t.Errorf("applyLLMDedup =\n  %+v\nwant\n  %+v", final, want)
	}
}
