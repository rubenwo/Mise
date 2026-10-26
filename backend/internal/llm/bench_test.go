//go:build e2e

// Scored model benchmark against a live Ollama server. Unlike the pass/fail
// e2e tests, it runs several scenarios per task and records timings,
// deterministic quality checks and the raw outputs for manual review.
//
//	MISE_OLLAMA_HOST=http://host:11434 MISE_E2E_MODEL=gemma4:26b MISE_E2E_THINK=0 \
//	MISE_BENCH_OUT=results.jsonl go test -tags e2e -v -run Benchmark ./internal/llm/
//
// Each run appends one JSON line to MISE_BENCH_OUT and unloads the model at
// the end so only one benchmark model occupies memory at a time.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rubenwo/mise/internal/models"
)

type benchResult struct {
	Model    string                    `json:"model"`
	Think    bool                      `json:"think"`
	VRAMGB   float64                   `json:"vram_gb"`
	TotalGB  float64                   `json:"total_gb"`
	LoadSecs float64                   `json:"load_secs"`
	Tasks    map[string]*bTask         `json:"tasks"`
	Recipes  map[string]*models.Recipe `json:"recipes"`
}

type bTask struct {
	Secs   float64  `json:"secs"`
	Score  float64  `json:"score"` // 0..1 from deterministic checks
	Issues []string `json:"issues,omitempty"`
	Note   string   `json:"note,omitempty"`
}

var allowedUnits = []string{"g", "kg", "ml", "l", "tsp", "tbsp", "", "clove", "pinch", "bunch", "sprig", "can", "slice", "stalk", "piece", "leaf", "sheet", "head", "stick"}

var (
	benchAnimalRe     = regexp.MustCompile(`\b(chicken|beef|pork|lamb|bacon|ham|guanciale|pancetta|sausage|fish|salmon|tuna|anchov\w*|shrimp|prawn\w*|egg|eggs|butter|milk|cream|cheese|parmesan|pecorino|mozzarella|ricotta|yogh?urt|honey|gelatin\w*|lard|ghee)\b`)
	benchGlutenRe     = regexp.MustCompile(`\b(flour|wheat|bread\w*|pasta|spaghetti|noodle\w*|couscous|barley|bulgur|semolina|panko|soy sauce|beer|seitan)\b`)
	benchGlutenSafeRe = regexp.MustCompile(`gluten[- ]free|tamari|rice noodle|rice flour|corn flour|cornflour|chickpea flour|almond flour|buckwheat`)
)

// checks scores a generated recipe; each failed check costs an equal share.
func benchChecks(r *models.Recipe, servings int, cuisines []string, extra func(*models.Recipe) []string) (float64, []string) {
	var issues []string
	total := 0
	check := func(ok bool, issue string) {
		total++
		if !ok {
			issues = append(issues, issue)
		}
	}
	check(r.Title != "" && r.Description != "", "missing title/description")
	check(servings == 0 || r.Servings == servings, fmt.Sprintf("servings %d, want %d", r.Servings, servings))
	check(len(cuisines) == 0 || slices.Contains(cuisines, r.CuisineType), "cuisine "+r.CuisineType+" not allowed")
	check(slices.Contains([]string{"easy", "medium", "hard"}, r.Difficulty), "difficulty "+r.Difficulty)
	check(len(r.Ingredients) >= 3 && len(r.Instructions) >= 4, fmt.Sprintf("%d ingredients, %d steps", len(r.Ingredients), len(r.Instructions)))

	var badUnits []string
	for _, ing := range r.Ingredients {
		if !slices.Contains(allowedUnits, strings.ToLower(strings.TrimSpace(ing.Unit))) {
			badUnits = append(badUnits, ing.Unit+" ("+ing.Name+")")
		}
	}
	check(len(badUnits) == 0, "units: "+strings.Join(badUnits, ", "))

	numbered := slices.ContainsFunc(r.Instructions, func(s string) bool {
		return regexp.MustCompile(`^\s*(step\s*)?\d+[.):]`).MatchString(strings.ToLower(s))
	})
	check(!numbered, "numbered steps")

	// Ingredient coverage: the last word of each ingredient name should
	// appear in the method ("chicken thigh" -> "thigh" or "chicken").
	method := strings.ToLower(strings.Join(r.Instructions, " "))
	var unused []string
	for _, ing := range r.Ingredients {
		words := strings.Fields(strings.ToLower(regexp.MustCompile(`\(.*?\)`).ReplaceAllString(ing.Name, "")))
		if len(words) == 0 {
			continue
		}
		used := slices.ContainsFunc(words, func(w string) bool {
			w = strings.TrimSuffix(strings.Trim(w, ",."), "s")
			return len(w) > 2 && strings.Contains(method, w)
		})
		if !used {
			unused = append(unused, ing.Name)
		}
	}
	check(len(unused) <= 1, "ingredients not used in method: "+strings.Join(unused, ", "))

	if extra != nil {
		for _, issue := range extra(r) {
			total++
			issues = append(issues, issue)
		}
		total++ // the extra block counts as at least one passed check
	}
	return 1 - float64(len(issues))/float64(total), issues
}

func ingredientText(r *models.Recipe) string {
	var parts []string
	for _, ing := range r.Ingredients {
		parts = append(parts, strings.ToLower(ing.Name+" "+ing.Notes))
	}
	return strings.Join(parts, "; ")
}

func TestBenchmark(t *testing.T) {
	model := e2eEnv("MISE_E2E_MODEL", "gemma4:26b")
	client := e2eClient(t, model)
	out := os.Getenv("MISE_BENCH_OUT")
	res := &benchResult{Model: model, Think: client.think, Tasks: map[string]*bTask{}, Recipes: map[string]*models.Recipe{}}
	host := os.Getenv("MISE_OLLAMA_HOST")
	t.Cleanup(func() { unloadModel(host, model) })

	// Warm-up: load the model once so task timings exclude loading.
	start := time.Now()
	if _, err := client.ChatJSON(context.Background(), []Message{{Role: "user", Content: `Reply {"ok": true}`}}, nil); err != nil {
		t.Fatalf("warm-up failed: %v", err)
	}
	res.LoadSecs = time.Since(start).Seconds()
	res.VRAMGB, res.TotalGB = loadedSize(host, model)
	t.Logf("%s think=%v loaded in %.1fs, %.1f/%.1f GB in VRAM", model, client.think, res.LoadSecs, res.VRAMGB, res.TotalGB)

	cuisines := AllCuisines(nil)
	o := &Orchestrator{
		pool:          &ClientPool{clients: []*Client{client}},
		tools:         []Tool{WebSearchTool, DBSearchTool},
		toolExecutor:  &stubTools{},
		maxIterations: 5,
	}
	generate := func(name, prompt string, avoid []string, servings int, extra func(*models.Recipe) []string) {
		events := make(chan SSEEvent, 100)
		finish := drain(events)
		start := time.Now()
		r, _, err := o.GenerateWithTag(context.Background(), prompt, avoid, events, "generation")
		finish()
		task := &bTask{Secs: time.Since(start).Seconds()}
		res.Tasks[name] = task
		if err != nil {
			task.Issues = []string{"error: " + err.Error()}
			return
		}
		res.Recipes[name] = r
		task.Score, task.Issues = benchChecks(r, servings, cuisines, extra)
		task.Note = r.Title
	}

	generate("gen_thai", BuildGeneratePrompt(models.GenerateRequest{CuisineType: "Thai", Servings: 2, AdditionalNotes: "weeknight, under 45 minutes total"}, nil),
		[]string{"Pad Thai", "Thai Green Curry"}, 2, func(r *models.Recipe) []string {
			if strings.EqualFold(r.Title, "Pad Thai") || strings.EqualFold(r.Title, "Thai Green Curry") {
				return []string{"repeated an existing title"}
			}
			if r.PrepTimeMinutes+r.CookTimeMinutes > 45 {
				return []string{fmt.Sprintf("total time %d > 45", r.PrepTimeMinutes+r.CookTimeMinutes)}
			}
			return nil
		})
	generate("gen_vegan", BuildGeneratePrompt(models.GenerateRequest{CuisineType: "Italian", Servings: 4, DietaryRestrictions: []string{"vegan"}}, nil),
		nil, 4, func(r *models.Recipe) []string {
			var issues []string
			// Judge each ingredient on its own: "vegan parmesan" is fine.
			plant := regexp.MustCompile(`vegan|plant|dairy-free|non-dairy|oat|soy|almond|cashew|coconut|nutritional`)
			for _, ing := range r.Ingredients {
				text := strings.ToLower(ing.Name + " " + ing.Notes)
				if benchAnimalRe.MatchString(text) && !plant.MatchString(text) {
					issues = append(issues, "animal product: "+ing.Name)
				}
			}
			if !slices.Contains(r.DietaryRestrictions, "vegan") {
				issues = append(issues, "vegan missing from dietary_restrictions")
			}
			return issues
		})
	generate("gen_glutenfree", BuildGeneratePrompt(models.GenerateRequest{CuisineType: "Mexican", Servings: 3, DietaryRestrictions: []string{"gluten-free"}, MaxPrepTime: 20}, nil),
		nil, 3, func(r *models.Recipe) []string {
			var issues []string
			text := ingredientText(r)
			for _, m := range benchGlutenRe.FindAllString(text, -1) {
				if !benchGlutenSafeRe.MatchString(text) {
					issues = append(issues, "gluten: "+m)
				}
			}
			if r.PrepTimeMinutes > 20 {
				issues = append(issues, fmt.Sprintf("prep %d > 20", r.PrepTimeMinutes))
			}
			return issues
		})
	generate("gen_background", BuildBackgroundGeneratePrompt("Ethiopian", cuisines, 1, 2, 4), nil, 4, nil)

	// Refine: change applied, everything else preserved.
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
	{
		events := make(chan SSEEvent, 100)
		finish := drain(events)
		start := time.Now()
		r, _, err := o.GenerateRefine(context.Background(), BuildRefinePrompt(base, "make it for 4 people and use smoked tofu instead of guanciale"), events)
		finish()
		task := &bTask{Secs: time.Since(start).Seconds()}
		res.Tasks["refine"] = task
		if err != nil {
			task.Issues = []string{"error: " + err.Error()}
		} else {
			res.Recipes["refine"] = r
			text := ingredientText(r)
			checks := map[string]bool{
				"servings not 4":          r.Servings == 4,
				"guanciale still present": !strings.Contains(text, "guanciale"),
				"no tofu":                 strings.Contains(text, "tofu"),
				"spaghetti not scaled ~400": slices.ContainsFunc(r.Ingredients, func(i models.Ingredient) bool {
					return strings.Contains(i.Name, "spaghetti") && i.Amount >= 350 && i.Amount <= 450
				}),
				"title lost carbonara":     strings.Contains(strings.ToLower(r.Title), "carbonara"),
				"steps don't mention tofu": strings.Contains(strings.ToLower(strings.Join(r.Instructions, " ")), "tofu"),
			}
			passed := 0
			for issue, ok := range checks {
				if ok {
					passed++
				} else {
					task.Issues = append(task.Issues, issue)
				}
			}
			task.Score = float64(passed) / float64(len(checks))
		}
	}

	// Import: faithful to the pasted Dutch recipe.
	{
		raw := `Oma's appeltaart
250g bloem, 150g roomboter, 100g suiker, 1 ei, snufje zout.
Vulling: 1kg goudrenetten, 75g rozijnen, 1 tl kaneel, 50g suiker.
Deeg maken, vorm bekleden, vullen, raster erop. 60 min op 175 graden.`
		events := make(chan SSEEvent, 100)
		finish := drain(events)
		start := time.Now()
		r, _, err := o.GenerateWithTag(context.Background(), BuildImportPrompt(raw), nil, events, "generation")
		finish()
		task := &bTask{Secs: time.Since(start).Seconds()}
		res.Tasks["import"] = task
		if err != nil {
			task.Issues = []string{"error: " + err.Error()}
		} else {
			res.Recipes["import"] = r
			text := ingredientText(r)
			has := func(amount float64, words ...string) bool {
				return slices.ContainsFunc(r.Ingredients, func(i models.Ingredient) bool {
					name := strings.ToLower(i.Name + " " + i.Notes)
					return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(name, w) }) &&
						(amount == 0 || (i.Amount >= amount*0.9 && i.Amount <= amount*1.1))
				})
			}
			checks := map[string]bool{
				"flour 250g":  has(250, "flour", "bloem"),
				"butter 150g": has(150, "butter", "roomboter"),
				// 1 kg, or a count of whole apples (~200 g each) per the unit rules.
				"apples 1kg": has(1000, "apple", "goudrenet") || has(1, "apple", "goudrenet") ||
					slices.ContainsFunc(r.Ingredients, func(i models.Ingredient) bool {
						return strings.Contains(strings.ToLower(i.Name+i.Notes), "apple") && i.Unit == "" && i.Amount >= 4 && i.Amount <= 7
					}),
				"raisins 75g":          has(75, "raisin", "rozijn"),
				"cinnamon":             has(0, "cinnamon", "kaneel"),
				"no invented extras":   len(r.Ingredients) <= 11,
				"175 degrees in steps": strings.Contains(strings.Join(r.Instructions, " "), "175"),
				"title kept":           strings.Contains(strings.ToLower(r.Title), "appeltaart") || strings.Contains(strings.ToLower(r.Title), "apple"),
			}
			_ = text
			passed := 0
			for issue, ok := range checks {
				if ok {
					passed++
				} else {
					task.Issues = append(task.Issues, issue)
				}
			}
			task.Score = float64(passed) / float64(len(checks))
		}
	}

	// Dedup arithmetic.
	{
		groups := [][]models.AggregatedIngredient{
			{{Name: "Parmesan", Amount: 100, Unit: "g"}, {Name: "parmesan", Amount: 0.5, Unit: "cup"}},
			{{Name: "Honey", Amount: 30, Unit: "g"}, {Name: "honey", Amount: 2, Unit: "tbsp"}},
			{{Name: "Butter", Amount: 50, Unit: "g"}, {Name: "butter", Amount: 3, Unit: "tbsp"}, {Name: "butter", Amount: 0.25, Unit: "cup"}},
			{{Name: "Olive oil", Amount: 100, Unit: "ml"}, {Name: "olive oil", Amount: 50, Unit: "g"}},
			{{Name: "Rice", Amount: 300, Unit: "g"}, {Name: "rice", Amount: 1, Unit: "cup"}},
		}
		ranges := []struct {
			min, max float64
		}{{130, 200}, {60, 85}, {130, 170}, {140, 165}, {450, 520}}
		start := time.Now()
		merged, err := o.DeduplicateIngredients(context.Background(), groups)
		task := &bTask{Secs: time.Since(start).Seconds()}
		res.Tasks["dedup"] = task
		if err != nil {
			task.Issues = []string{"error: " + err.Error()}
		} else {
			passed := 0
			for i, want := range ranges {
				got := merged[i]
				ok := got.Amount >= want.min && got.Amount <= want.max && (got.Unit == "g" || got.Unit == "ml")
				if ok {
					passed++
				} else {
					task.Issues = append(task.Issues, fmt.Sprintf("%s %.0f %s (want %.0f-%.0f)", got.Name, got.Amount, got.Unit, want.min, want.max))
				}
			}
			task.Score = float64(passed) / float64(len(ranges))
		}
	}

	// Translation with the supermarket prompt (TranslateGemma is benchmarked
	// separately by TestE2ETranslate).
	{
		cases := map[string][]string{
			"spring onion": {"lente-ui", "lenteui", "bosui"}, "bell pepper": {"paprika"}, "heavy cream": {"slagroom"},
			"chickpeas": {"kikkererwten"}, "ground beef": {"rundergehakt", "gehakt"}, "cilantro": {"koriander", "verse koriander"},
			"eggplant": {"aubergine"}, "brown sugar": {"bruine suiker", "basterdsuiker"}, "chicken thigh": {"kippendij", "kippendijen", "kipdij", "kipdijen"},
			"baking soda": {"zuiveringszout", "baking soda"}, "zucchini": {"courgette"}, "puff pastry": {"bladerdeeg"},
			"powdered sugar": {"poedersuiker"}, "sour cream": {"zure room"}, "pork belly": {"buikspek", "varkensbuik"},
			"leek": {"prei"}, "coconut milk": {"kokosmelk"}, "sweet potato": {"zoete aardappel"}, "fennel": {"venkel"},
			"mushrooms": {"champignons", "paddenstoelen"}, "cornstarch": {"maizena", "maïzena", "maïszetmeel"},
			"shallot": {"sjalot", "sjalotten"}, "lemongrass": {"citroengras"}, "tofu": {"tofu"},
		}
		task := &bTask{}
		res.Tasks["translate"] = task
		start := time.Now()
		passed := 0
		for en, accepted := range cases {
			resp, err := client.ChatJSON(context.Background(), []Message{
				{Role: "system", Content: `Translate an English recipe ingredient into the Dutch product name a shopper would type into a supermarket search box. Use the everyday supermarket term, singular or plural as it is sold, without amounts, preparation or brand. Examples: "minced garlic" -> "knoflook", "self-raising flour" -> "zelfrijzend bakmeel", "scallops" -> "coquilles", "double cream" -> "slagroom". Reply as {"translation": "<term>"}.`},
				{Role: "user", Content: en},
			}, TranslationSchema)
			var got struct {
				Translation string `json:"translation"`
			}
			if err == nil {
				_ = json.Unmarshal([]byte(resp.Message.Content), &got)
			}
			term := strings.ToLower(strings.TrimSpace(got.Translation))
			if slices.Contains(accepted, term) {
				passed++
			} else {
				task.Issues = append(task.Issues, en+" -> "+term)
			}
		}
		task.Secs = time.Since(start).Seconds() / float64(len(cases))
		task.Score = float64(passed) / float64(len(cases))
		task.Note = "secs is per item"
	}

	for name, task := range res.Tasks {
		t.Logf("%-15s %5.1fs score=%.2f %s %v", name, task.Secs, task.Score, task.Note, task.Issues)
	}
	if out != "" {
		data, _ := json.Marshal(res)
		f, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		_, _ = f.Write(append(data, '\n'))
	}
}

// loadedSize reports how much of a loaded model sits in VRAM vs in total.
func loadedSize(host, model string) (vram, total float64) {
	resp, err := http.Get(host + "/api/ps")
	if err != nil {
		return 0, 0
	}
	defer resp.Body.Close()
	var ps struct {
		Models []struct {
			Name     string `json:"name"`
			Size     int64  `json:"size"`
			SizeVRAM int64  `json:"size_vram"`
		} `json:"models"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ps)
	for _, m := range ps.Models {
		if m.Name == model {
			return float64(m.SizeVRAM) / 1e9, float64(m.Size) / 1e9
		}
	}
	return 0, 0
}

// unloadModel frees the model's memory on the server.
func unloadModel(host, model string) {
	body, _ := json.Marshal(map[string]any{"model": model, "keep_alive": 0})
	resp, err := http.Post(host+"/api/generate", "application/json", bytes.NewReader(body))
	if err == nil {
		resp.Body.Close()
	}
}
