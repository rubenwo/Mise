package llm

import (
	"slices"
	"testing"

	"github.com/rubenwo/mise/internal/models"
)

func TestDropContradictedDiets(t *testing.T) {
	tests := []struct {
		name        string
		labels      []string
		ingredients []string
		wantKept    []string
	}{
		{"couscous is not gluten-free", []string{"gluten-free"}, []string{"white fish fillet", "couscous"}, nil},
		{"soy sauce is not gluten-free", []string{"gluten-free", "dairy-free"}, []string{"chicken thigh", "soy sauce"}, []string{"dairy-free"}},
		{"tamari and rice noodles are fine", []string{"gluten-free"}, []string{"tamari", "rice noodles", "corn tortilla", "chickpea flour"}, []string{"gluten-free"}},
		{"fish sauce is not vegetarian", []string{"vegetarian", "vegan"}, []string{"tofu", "fish sauce"}, nil},
		{"goat meat is meat", []string{"vegetarian"}, []string{"goat meat chunk"}, nil},
		{"goat cheese is vegetarian", []string{"vegetarian"}, []string{"goat cheese", "spinach"}, []string{"vegetarian"}},
		{"eggplant is not egg", []string{"vegan"}, []string{"eggplant", "coconut milk", "peanut butter"}, []string{"vegan"}},
		{"egg and butter are not vegan", []string{"vegan", "vegetarian"}, []string{"egg", "butter"}, []string{"vegetarian"}},
		{"vegan sausage is vegan", []string{"vegan"}, []string{"vegan sausage", "oat milk"}, []string{"vegan"}},
		{"niter kibbeh is dairy", []string{"dairy-free"}, []string{"niter kibbeh"}, nil},
		{"either-or keeps the label", []string{"vegetarian", "dairy-free"}, []string{"chicken or vegetable stock", "mayonnaise or greek yogurt"}, []string{"vegetarian", "dairy-free"}},
		{"substitutes and lookalikes", []string{"vegan"}, []string{"fish sauce substitute", "butter lettuce"}, []string{"vegan"}},
		{"rice vermicelli is gluten-free", []string{"gluten-free"}, []string{"vermicelli noodles"}, []string{"gluten-free"}},
		{"pescatarian label untouched", []string{"pescatarian"}, []string{"salmon"}, []string{"pescatarian"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := models.Recipe{Title: tt.name, DietaryRestrictions: slices.Clone(tt.labels)}
			for _, n := range tt.ingredients {
				r.Ingredients = append(r.Ingredients, models.Ingredient{Name: n})
			}
			DropContradictedDiets(&r)
			if len(r.DietaryRestrictions)+len(tt.wantKept) > 0 && !slices.Equal(r.DietaryRestrictions, tt.wantKept) {
				t.Errorf("labels = %v, want %v", r.DietaryRestrictions, tt.wantKept)
			}
		})
	}
}
