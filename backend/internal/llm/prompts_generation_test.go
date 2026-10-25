package llm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rubenwo/mise/internal/models"
)

func TestGenerationSystemPrompt_OnlyMentionsOfferedTools(t *testing.T) {
	withoutEdamam := GenerationSystemPrompt([]Tool{WebSearchTool, DBSearchTool})
	if strings.Contains(withoutEdamam, "edamam_search") {
		t.Error("prompt mentions edamam_search although the tool is not offered")
	}
	if !strings.Contains(withoutEdamam, "db_search") || !strings.Contains(withoutEdamam, "web_search") {
		t.Error("prompt should describe the offered tools")
	}

	withEdamam := GenerationSystemPrompt([]Tool{WebSearchTool, DBSearchTool, EdamamSearchTool})
	if !strings.Contains(withEdamam, "edamam_search") {
		t.Error("prompt should mention edamam_search when it is offered")
	}

	noTools := GenerationSystemPrompt(nil)
	if noTools != SystemPrompt() {
		t.Error("with no tools the base system prompt should be unchanged")
	}
}

func TestAvoidTitlesInstruction(t *testing.T) {
	if AvoidTitlesInstruction(nil) != "" {
		t.Error("no titles should add nothing")
	}
	titles := make([]string, 150)
	for i := range titles {
		titles[i] = "T"
	}
	prompt := AvoidTitlesInstruction(titles)
	if got := strings.Count(prompt, "T;") + 1; got != 100 {
		t.Errorf("avoid list has %d titles, want 100", got)
	}
}

func TestBuildGeneratePrompt(t *testing.T) {
	prompt := BuildGeneratePrompt(models.GenerateRequest{
		CuisineType:         "Thai",
		Servings:            2,
		DietaryRestrictions: []string{"vegan"},
		AdditionalNotes:     "  spicy  ",
	}, nil)
	for _, want := range []string{"Thai cuisine", `"servings" to 2`, "vegan", "The user also asked for: spicy", mealRules} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "edamam") {
		t.Error("user prompt must not hard-code tool instructions")
	}
}

func TestBuildRefinePrompt_EmbedsRecipeAsJSON(t *testing.T) {
	prompt := BuildRefinePrompt(models.Recipe{
		Title:       "Pad Thai",
		Servings:    2,
		Ingredients: []models.Ingredient{{Name: "rice noodles", Amount: 200, Unit: "g"}},
	}, "make it vegetarian")
	start, end := strings.Index(prompt, "{"), strings.LastIndex(prompt, "}")
	var embedded map[string]any
	if err := json.Unmarshal([]byte(prompt[start:end+1]), &embedded); err != nil {
		t.Fatalf("embedded recipe is not valid JSON: %v", err)
	}
	if embedded["title"] != "Pad Thai" {
		t.Errorf("embedded title = %v", embedded["title"])
	}
	if !strings.Contains(prompt, "make it vegetarian") {
		t.Error("feedback missing from prompt")
	}
}

func TestSystemPromptListsAllowedValues(t *testing.T) {
	for _, list := range [][]string{RecipeUnits, DietaryLabels, RecipeTags} {
		for _, v := range list {
			if v != "" && !strings.Contains(systemPrompt, v) {
				t.Errorf("system prompt does not mention allowed value %q", v)
			}
		}
	}
}

func TestSchemasAreValidJSON(t *testing.T) {
	for _, s := range []*Schema{RecipeSchema(nil), RecipeSchema([]string{"Thai"}), DedupSchema, ScanSchema, RankSchema} {
		var v map[string]any
		if err := json.Unmarshal(s.JSON, &v); err != nil {
			t.Errorf("%s schema is not valid JSON: %v", s.Name, err)
		}
		if v["type"] != "object" {
			t.Errorf("%s schema top-level type = %v, want object", s.Name, v["type"])
		}
	}
}

func TestRecipeSchema_CuisineEnum(t *testing.T) {
	var v struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(RecipeSchema([]string{"Thai", "Dutch"}).JSON, &v); err != nil {
		t.Fatal(err)
	}
	if got := v.Properties["cuisine_type"].Enum; len(got) != 2 {
		t.Errorf("cuisine enum = %v", got)
	}
	if got := v.Properties["difficulty"].Enum; len(got) != 3 {
		t.Errorf("difficulty enum = %v", got)
	}
	if len(v.Required) != 11 {
		t.Errorf("required fields = %d, want 11", len(v.Required))
	}
}
