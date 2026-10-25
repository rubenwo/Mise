//go:build e2e

// End-to-end tests against a live Ollama server. They exercise the real
// prompts, schemas and orchestrator with stubbed tools, so no database or
// internet access is needed. Run with:
//
//	MISE_OLLAMA_HOST=http://host:11434 go test -tags e2e -v -run E2E ./internal/llm/
//
// Optional: MISE_E2E_MODEL (default gemma4:26b), MISE_E2E_TRANSLATE_MODEL
// (default translategemma:4b), MISE_E2E_NUM_CTX (default 16384).
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rubenwo/mise/internal/models"
)

func e2eEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func e2eClient(t *testing.T, model string) *Client {
	t.Helper()
	host := os.Getenv("MISE_OLLAMA_HOST")
	if host == "" {
		t.Skip("MISE_OLLAMA_HOST not set")
	}
	numCtx, _ := strconv.Atoi(e2eEnv("MISE_E2E_NUM_CTX", "16384"))
	return NewClient(ProviderConfig{
		Host:         host,
		Model:        model,
		ProviderType: ProviderTypeOllama,
		Timeout:      5 * time.Minute,
		Tags:         []string{"generation", "deduplication", "translation", "search"},
		NumCtx:       numCtx,
		Think:        os.Getenv("MISE_E2E_THINK") == "1",
	})
}

// stubTools records tool calls and returns canned results.
type stubTools struct {
	mu    sync.Mutex
	calls []string
}

func (s *stubTools) Execute(_ context.Context, name string, args json.RawMessage) (string, error) {
	s.mu.Lock()
	s.calls = append(s.calls, name+" "+string(args))
	s.mu.Unlock()
	switch name {
	case "db_search":
		return `[{"id":1,"title":"Pad Thai","description":"Stir-fried rice noodles with tamarind","cuisine_type":"Thai"},{"id":2,"title":"Thai Green Curry","description":"Coconut curry with chicken","cuisine_type":"Thai"}]`, nil
	case "web_search":
		return `[]`, nil
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

func drain(events chan SSEEvent) (done func() []SSEEvent) {
	var got []SSEEvent
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for e := range events {
			got = append(got, e)
		}
	}()
	return func() []SSEEvent { close(events); wg.Wait(); return got }
}

var metricUnits = []string{"g", "kg", "ml", "l", "tsp", "tbsp", "", "clove", "pinch", "bunch", "sprig", "can", "slice", "stalk", "piece", "leaf", "sheet", "head"}

func checkRecipe(t *testing.T, r *models.Recipe, servings int, cuisines []string) {
	t.Helper()
	if r.Title == "" || r.Description == "" {
		t.Errorf("missing title/description: %+v", r)
	}
	if servings > 0 && r.Servings != servings {
		t.Errorf("servings = %d, want %d", r.Servings, servings)
	}
	if len(cuisines) > 0 && !slices.Contains(cuisines, r.CuisineType) {
		t.Errorf("cuisine_type %q not in allowed list", r.CuisineType)
	}
	if !slices.Contains([]string{"easy", "medium", "hard"}, r.Difficulty) {
		t.Errorf("difficulty = %q", r.Difficulty)
	}
	if len(r.Ingredients) < 3 || len(r.Instructions) < 3 {
		t.Errorf("too few ingredients (%d) or steps (%d)", len(r.Ingredients), len(r.Instructions))
	}
	for _, ing := range r.Ingredients {
		if !slices.Contains(metricUnits, strings.ToLower(ing.Unit)) {
			t.Errorf("unexpected unit %q for %q", ing.Unit, ing.Name)
		}
		if ing.Amount < 0 {
			t.Errorf("negative amount for %q", ing.Name)
		}
	}
	for i, step := range r.Instructions {
		if len(step) > 2 && step[0] >= '0' && step[0] <= '9' && (step[1] == '.' || step[1] == ')') {
			t.Errorf("step %d is numbered: %q", i+1, step)
		}
	}
}

func logRecipe(t *testing.T, r *models.Recipe) {
	t.Helper()
	data, _ := json.MarshalIndent(r, "", "  ")
	t.Logf("recipe:\n%s", data)
}

func TestE2EGenerate(t *testing.T) {
	model := e2eEnv("MISE_E2E_MODEL", "gemma4:26b")
	client := e2eClient(t, model)
	tools := &stubTools{}
	o := &Orchestrator{
		pool:          &ClientPool{clients: []*Client{client}},
		tools:         []Tool{WebSearchTool, DBSearchTool}, // Edamam not configured
		toolExecutor:  tools,
		maxIterations: 5,
	}

	cuisines := AllCuisines(nil)
	prompt := BuildGeneratePrompt(models.GenerateRequest{CuisineType: "Thai", Servings: 2, AdditionalNotes: "weeknight, under 45 minutes total"}, nil)
	events := make(chan SSEEvent, 100)
	finish := drain(events)

	start := time.Now()
	recipe, _, err := o.GenerateWithTag(context.Background(), prompt, []string{"Pad Thai", "Thai Green Curry"}, events, "generation")
	evs := finish()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("generate failed after %s: %v", elapsed, err)
	}
	reformatted := slices.ContainsFunc(evs, func(e SSEEvent) bool { return strings.HasPrefix(e.Message, "Reformatting") })
	t.Logf("model=%s elapsed=%s tool calls=%q reformat-fallback=%v", model, elapsed.Round(time.Millisecond), tools.calls, reformatted)
	logRecipe(t, recipe)

	checkRecipe(t, recipe, 2, cuisines)
	if slices.ContainsFunc(tools.calls, func(c string) bool { return strings.HasPrefix(c, "edamam_search") }) {
		t.Error("model called edamam_search although it was not offered")
	}
	if strings.EqualFold(recipe.Title, "Pad Thai") || strings.EqualFold(recipe.Title, "Thai Green Curry") {
		t.Errorf("model reproduced an existing title: %q", recipe.Title)
	}
}

func TestE2ERefine(t *testing.T) {
	model := e2eEnv("MISE_E2E_MODEL", "gemma4:26b")
	client := e2eClient(t, model)
	o := &Orchestrator{pool: &ClientPool{clients: []*Client{client}}}

	base := models.Recipe{
		Title: "Spaghetti Carbonara", Description: "Roman pasta with egg, cheese and guanciale.", CuisineType: "Italian",
		PrepTimeMinutes: 10, CookTimeMinutes: 15, Servings: 2, Difficulty: "medium",
		Ingredients: []models.Ingredient{
			{Name: "spaghetti", Amount: 200, Unit: "g"}, {Name: "guanciale", Amount: 100, Unit: "g"},
			{Name: "egg yolk", Amount: 3, Unit: ""}, {Name: "pecorino romano", Amount: 40, Unit: "g"},
			{Name: "black pepper", Amount: 1, Unit: "tsp"},
		},
		Instructions: []string{"Boil the spaghetti in salted water.", "Crisp the guanciale in a dry pan.", "Whisk yolks with pecorino and pepper.", "Toss pasta with guanciale off the heat, then the egg mixture and a splash of pasta water."},
	}
	events := make(chan SSEEvent, 100)
	finish := drain(events)
	start := time.Now()
	recipe, _, err := o.GenerateRefine(context.Background(), BuildRefinePrompt(base, "make it vegetarian"), events)
	finish()
	if err != nil {
		t.Fatalf("refine failed: %v", err)
	}
	t.Logf("model=%s elapsed=%s", model, time.Since(start).Round(time.Millisecond))
	logRecipe(t, recipe)

	checkRecipe(t, recipe, 2, nil)
	for _, ing := range recipe.Ingredients {
		if strings.Contains(strings.ToLower(ing.Name), "guanciale") {
			t.Error("guanciale still present after vegetarian refine")
		}
	}
	if !strings.Contains(strings.ToLower(recipe.Title), "carbonara") {
		t.Errorf("title changed unexpectedly: %q", recipe.Title)
	}
}

func TestE2EImport(t *testing.T) {
	model := e2eEnv("MISE_E2E_MODEL", "gemma4:26b")
	client := e2eClient(t, model)
	tools := &stubTools{}
	o := &Orchestrator{pool: &ClientPool{clients: []*Client{client}}, tools: []Tool{WebSearchTool, DBSearchTool}, toolExecutor: tools, maxIterations: 5}

	raw := `Oma's appeltaart
250g bloem, 150g roomboter, 100g suiker, 1 ei, snufje zout.
Vulling: 1kg goudrenetten, 75g rozijnen, 1 tl kaneel, 50g suiker.
Deeg maken, vorm bekleden, vullen, raster erop. 60 min op 175 graden.`
	events := make(chan SSEEvent, 100)
	finish := drain(events)
	start := time.Now()
	recipe, _, err := o.GenerateWithTag(context.Background(), BuildImportPrompt(raw), nil, events, "generation")
	finish()
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	t.Logf("model=%s elapsed=%s tool calls=%q", model, time.Since(start).Round(time.Millisecond), tools.calls)
	logRecipe(t, recipe)

	checkRecipe(t, recipe, 0, nil)
	names := strings.ToLower(fmt.Sprint(recipe.Ingredients))
	for _, want := range []string{"rais", "cinnamon|kaneel", "apple|goudrenet"} {
		if !slices.ContainsFunc(strings.Split(want, "|"), func(w string) bool { return strings.Contains(names, w) }) {
			t.Errorf("imported ingredients lost %q: %s", want, names)
		}
	}
}

func TestE2EDedup(t *testing.T) {
	model := e2eEnv("MISE_E2E_DEDUP_MODEL", e2eEnv("MISE_E2E_MODEL", "gemma4:26b"))
	client := e2eClient(t, model)
	client.tags = []string{"deduplication"}
	o := &Orchestrator{pool: &ClientPool{clients: []*Client{client}}}

	groups := [][]models.AggregatedIngredient{
		{{Name: "Parmesan", Amount: 100, Unit: "g", Recipes: []string{"A"}}, {Name: "parmesan", Amount: 0.5, Unit: "cup", Recipes: []string{"B"}}},
		{{Name: "Honey", Amount: 30, Unit: "g", Recipes: []string{"A"}}, {Name: "honey", Amount: 2, Unit: "tbsp", Recipes: []string{"C"}}},
		{{Name: "Spinach", Amount: 200, Unit: "g", Recipes: []string{"B"}}, {Name: "spinach", Amount: 2, Unit: "cup", Recipes: []string{"C"}}},
	}
	start := time.Now()
	merged, err := o.DeduplicateIngredients(context.Background(), groups)
	if err != nil {
		t.Fatalf("dedup failed: %v", err)
	}
	t.Logf("model=%s elapsed=%s merged=%+v", model, time.Since(start).Round(time.Millisecond), merged)
	if len(merged) != len(groups) {
		t.Fatalf("got %d items, want %d", len(merged), len(groups))
	}
	// Plausible ranges: parmesan 100g + ~50g; honey 30g + ~42g; spinach 200g + ~60g.
	ranges := []struct {
		name     string
		min, max float64
	}{{"parmesan", 120, 230}, {"honey", 55, 90}, {"spinach", 220, 320}}
	for i, want := range ranges {
		got := merged[i]
		if !strings.Contains(strings.ToLower(got.Name), want.name) || got.Unit != "g" || got.Amount < want.min || got.Amount > want.max {
			t.Errorf("item %d = %+v, want %s in g between %.0f and %.0f", i, got, want.name, want.min, want.max)
		}
	}
}

func TestE2ETranslate(t *testing.T) {
	model := e2eEnv("MISE_E2E_TRANSLATE_MODEL", "translategemma:4b")
	client := e2eClient(t, model)

	// Expected Albert Heijn search terms; any listed variant is accepted.
	cases := map[string][]string{
		"spring onion":  {"lente-ui", "lenteui", "bosui"},
		"bell pepper":   {"paprika"},
		"heavy cream":   {"slagroom"},
		"chickpeas":     {"kikkererwten"},
		"ground beef":   {"rundergehakt", "gehakt", "rundergehakt"},
		"cilantro":      {"koriander"},
		"eggplant":      {"aubergine"},
		"brown sugar":   {"bruine suiker", "basterdsuiker"},
		"chicken thigh": {"kippendij", "kipdij"},
		"baking soda":   {"zuiveringszout", "baking soda"},
		"zucchini":      {"courgette"},
		"puff pastry":   {"bladerdeeg"},
	}

	// Compare the new TranslateGemma template with the previous JSON-mode prompt.
	oldPrompt := func(text string) (string, error) {
		resp, err := client.ChatJSON(context.Background(), []Message{
			{Role: "system", Content: `You translate food ingredient names from English to Dutch. Reply with ONLY valid JSON in this exact format: {"translation":"<result>"}`},
			{Role: "user", Content: text},
		}, nil)
		if err != nil {
			return "", err
		}
		var r struct {
			Translation string `json:"translation"`
		}
		if err := json.Unmarshal([]byte(resp.Message.Content), &r); err != nil {
			return "PARSE ERROR: " + resp.Message.Content, nil
		}
		return r.Translation, nil
	}
	newPrompt := func(text string) (string, error) {
		prompt := "You are a professional English (en) to Dutch (nl) translator. " +
			"Your goal is to accurately convey the meaning and nuances of the original English text " +
			"while adhering to Dutch grammar, vocabulary, and cultural sensitivities.\n" +
			"Produce only the Dutch translation, without any additional explanations or commentary. " +
			"Please translate the following English text into Dutch:\n\n\n" + text
		resp, err := client.Chat(context.Background(), []Message{{Role: "user", Content: prompt}}, nil)
		if err != nil {
			return "", err
		}
		s := strings.TrimSpace(resp.Message.Content)
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[:i]
		}
		return strings.TrimSuffix(strings.Trim(s, `"'`), "."), nil
	}

	// Domain-framed prompt for a general instruction model: ask for the term a
	// shopper would type into the supermarket search box. Examples are
	// deliberately not among the test cases.
	genModel := e2eEnv("MISE_E2E_MODEL", "gemma4:26b")
	genClient := e2eClient(t, genModel)
	ctxPrompt := func(text string) (string, error) {
		resp, err := genClient.ChatJSON(context.Background(), []Message{
			{Role: "system", Content: `Translate an English recipe ingredient into the Dutch product name a shopper would type into the Albert Heijn search box. Use the everyday Dutch supermarket term, singular or plural as it is sold, without amounts, preparation or brand. Examples: "minced garlic" -> "knoflook", "self-raising flour" -> "zelfrijzend bakmeel", "scallops" -> "coquilles", "double cream" -> "slagroom". Reply as {"translation": "<term>"}.`},
			{Role: "user", Content: text},
		}, TranslationSchema)
		if err != nil {
			return "", err
		}
		var r struct {
			Translation string `json:"translation"`
		}
		if err := json.Unmarshal([]byte(resp.Message.Content), &r); err != nil {
			return "PARSE ERROR: " + resp.Message.Content, nil
		}
		return r.Translation, nil
	}

	score := func(name string, fn func(string) (string, error)) int {
		ok := 0
		start := time.Now()
		for en, accepted := range cases {
			got, err := fn(en)
			if err != nil {
				t.Fatalf("%s translate %q: %v", name, en, err)
			}
			// Exact match: the result is used verbatim as a search query.
			hit := slices.Contains(accepted, strings.ToLower(strings.TrimSpace(got)))
			if hit {
				ok++
			}
			t.Logf("%-4s %-15s -> %-25q ok=%v", name, en, got, hit)
		}
		t.Logf("%s: %d/%d acceptable in %s", name, ok, len(cases), time.Since(start).Round(time.Millisecond))
		return ok
	}
	score("old", oldPrompt)
	score("new", newPrompt)
	score("ctx", ctxPrompt)
}

func TestE2EEmbed(t *testing.T) {
	c := e2eClient(t, e2eEnv("MISE_E2E_EMBED_MODEL", "embeddinggemma"))
	vecs, err := c.Embed(context.Background(), []string{"Chicken laksa with rice noodles", "Beef stew with potatoes"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 || len(vecs[0]) == 0 || len(vecs[0]) != len(vecs[1]) {
		t.Fatalf("got %d vectors of lengths %d/%d", len(vecs), len(vecs[0]), len(vecs[len(vecs)-1]))
	}
	t.Logf("%d-dimensional embeddings", len(vecs[0]))
}
