package handlers

import (
	"slices"
	"strings"
	"testing"

	"github.com/rubenwo/mise/internal/models"
)

func testRecipe(id int, title, cuisine string, minutes int, ingredients ...string) models.Recipe {
	r := models.Recipe{ID: id, Title: title, CuisineType: cuisine, CookTimeMinutes: minutes}
	for _, n := range ingredients {
		r.Ingredients = append(r.Ingredients, models.Ingredient{Name: n})
	}
	return r
}

func ids(rs []models.Recipe) []int {
	var out []int
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

var testLibrary = []models.Recipe{
	testRecipe(1, "Chicken Laksa", "Malaysian", 45, "chicken thigh", "laksa paste", "coconut milk", "rice noodles"),
	testRecipe(2, "Mushroom Risotto", "Italian", 40, "arborio rice", "chestnut mushrooms", "parmesan", "butter", "onion", "white wine", "stock"),
	testRecipe(3, "Tomato Pasta", "Italian", 20, "spaghetti", "tomatoes", "garlic", "basil"),
	testRecipe(4, "Chickpea Curry", "Indian", 30, "chickpea", "onion", "tomato", "garam masala", "coconut milk", "rice", "spinach", "ginger", "garlic", "cumin", "coriander", "chilli"),
	testRecipe(5, "Shakshuka", "Middle Eastern", 25, "eggs", "tomatoes", "pepper", "onion", "cumin"),
}

func TestShortlistAppliesDietAndAvoidStrictly(t *testing.T) {
	got, relaxed := shortlist(testLibrary, libraryQuery{Dietary: []string{"vegetarian"}, Avoid: []string{"mushrooms"}}, nil, 10)
	if want := []int{3, 4, 5}; !slices.Equal(ids(got), want) {
		t.Errorf("got %v, want %v (no chicken, no mushrooms)", ids(got), want)
	}
	if len(relaxed) != 0 {
		t.Errorf("relaxed %v without need", relaxed)
	}
	if got, _ := shortlist(testLibrary, libraryQuery{Dietary: []string{"vegan"}, Avoid: []string{"tomatoes"}}, nil, 10); len(got) != 0 {
		t.Errorf("hard filters were relaxed: got %v", ids(got))
	}
}

func TestShortlistRelaxesSoftFiltersInOrder(t *testing.T) {
	q := libraryQuery{Cuisine: "Italian", MaxMinutes: 15, Include: []string{"tomatoes"}}
	got, relaxed := shortlist(testLibrary, q, nil, 10)
	if want := []int{3}; !slices.Equal(ids(got), want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
	if want := []string{"the 15-minute limit"}; !slices.Equal(relaxed, want) {
		t.Errorf("relaxed %v, want %v", relaxed, want)
	}
	// Each requirement is dropped on its own: "smoky" goes, chickpeas stay.
	got, relaxed = shortlist(testLibrary, libraryQuery{Include: []string{"chickpeas", "smoky"}}, nil, 10)
	if want := []int{4}; !slices.Equal(ids(got), want) {
		t.Errorf("got %v, want %v", ids(got), want)
	}
	if want := []string{"smoky"}; !slices.Equal(relaxed, want) {
		t.Errorf("relaxed %v, want %v", relaxed, want)
	}
	if note := relaxedNote([]string{"the 15-minute limit", "the Thai cuisine", "tofu"}); !strings.Contains(note, "the 15-minute limit, the Thai cuisine and tofu") {
		t.Errorf("note = %q", note)
	}
}

func TestShortlistOrdersBySearchScoreAndIngredientBudget(t *testing.T) {
	scores := map[int]float64{3: 0.30, 4: 0.35, 5: 0.10}
	got, _ := shortlist(testLibrary, libraryQuery{Dietary: []string{"vegetarian"}, Avoid: []string{"mushroom"}}, scores, 2)
	if want := []int{4, 3}; !slices.Equal(ids(got), want) {
		t.Errorf("by score: got %v, want %v", ids(got), want)
	}
	// 12 ingredients against a budget of 8 costs 0.08: the curry drops below the pasta.
	got, _ = shortlist(testLibrary, libraryQuery{Dietary: []string{"vegetarian"}, Avoid: []string{"mushroom"}, MaxIngredients: 8}, scores, 2)
	if want := []int{3, 4}; !slices.Equal(ids(got), want) {
		t.Errorf("with ingredient budget: got %v, want %v", ids(got), want)
	}
}

func TestMentionsMatchesPluralsWithoutOverreaching(t *testing.T) {
	r := testRecipe(1, "Tomato Risotto", "Italian", 30, "chestnut mushroom", "egg", "arborio rice", "black olive", "chickpea")
	for _, food := range []string{"mushrooms", "eggs", "tomatoes", "olives", "chickpeas", "rice", "Rice"} {
		if !mentions(&r, food) {
			t.Errorf("mentions(%q) = false", food)
		}
	}
	ricotta := testRecipe(2, "Gnocchi", "Italian", 30, "ricotta")
	if mentions(&ricotta, "rice") {
		t.Error(`"rice" matched ricotta`)
	}
	if pad := testRecipe(3, "Pad Thai", "Thai", 20, "rice noodles", "fish sauce"); mentions(&pad, "fish") {
		t.Error(`"fish" matched fish sauce`)
	}
	if !mentions(&testLibrary[2], "pasta") || !mentions(&testLibrary[0], "noodles") {
		t.Error("food kinds: spaghetti should count as pasta, rice noodles as noodles")
	}
}

func TestFormatConversationKeepsEarlierTurns(t *testing.T) {
	got := formatConversation([]models.ChatMessage{
		{Role: "user", Content: "vegetarian pasta"},
		{Role: "assistant", Content: "Try the tomato pasta."},
		{Role: "user", Content: "something lighter"},
	})
	for _, want := range []string{"User: vegetarian pasta", "Mise: Try the tomato pasta.", `latest message: "something lighter"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
