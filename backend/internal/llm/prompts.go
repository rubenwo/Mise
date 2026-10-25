package llm

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/rubenwo/mise/internal/models"
)

// KnownCuisines is the canonical list of cuisines the app supports.
// It is used to seed cuisine counts so that cuisines with zero recipes
// are treated as the most underrepresented when balancing the library.
var KnownCuisines = []string{
	"American", "Argentine", "Brazilian", "British", "Caribbean",
	"Chinese", "Dutch", "Eastern European", "Ethiopian", "Filipino",
	"French", "German", "Indian", "Italian", "Japanese", "Korean",
	"Malaysian", "Mediterranean", "Mexican", "Middle Eastern", "Moroccan", "Peruvian",
	"Scandinavian", "Spanish", "Thai", "Turkish", "Vietnamese",
}

// cuisineAliases maps non-canonical cuisine names (lowercase) to their
// canonical form. Keys are matched after the qualifier stripping in
// NormalizeCuisine, so "Malaysian Peranakan (Nyonya)" only needs
// "malaysian peranakan".
var cuisineAliases = map[string]string{
	// All Malay, Nyonya and Peranakan variants are "Malaysian".
	"malay":               "Malaysian",
	"malaysian":           "Malaysian",
	"nyonya":              "Malaysian",
	"nonya":               "Malaysian",
	"peranakan":           "Malaysian",
	"nyonya peranakan":    "Malaysian",
	"nonya peranakan":     "Malaysian",
	"malay/nyonya":        "Malaysian",
	"nyonya/malay":        "Malaysian",
	"malay-nyonya":        "Malaysian",
	"malaysian nyonya":    "Malaysian",
	"malaysian/nyonya":    "Malaysian",
	"malaysian peranakan": "Malaysian",

	// Generic pan-Asian dishes.
	"asian":              "Asian Fusion",
	"asian fusion":       "Asian Fusion",
	"asian-fusion":       "Asian Fusion",
	"pan-asian":          "Asian Fusion",
	"korean-thai fusion": "Asian Fusion",

	// Named fusions that have a better home.
	"japanese-peruvian":             "Nikkei",
	"japanese-peruvian fusion":      "Nikkei",
	"nikkei":                        "Nikkei",
	"thai-chinese fusion":           "Thai",
	"greek mediterranean":           "Greek",
	"italian-mediterranean fusion":  "Italian",
	"british-south american fusion": "Fusion",
	"sichuan":                       "Szechuan",
}

var (
	// cuisineParenthetical matches a trailing note like " (Nyonya)".
	cuisineParenthetical = regexp.MustCompile(`\s*\(.*\)$`)
	// cuisineQualifier matches suffixes models add to a cuisine name, e.g.
	// "Thai-inspired", "Korean style", "Vietnamese-Asian Fusion".
	cuisineQualifier = regexp.MustCompile(`[-\s]+(inspired|style|asian fusion|asian)$`)
)

// NormalizeCuisine maps a model-produced cuisine label to its canonical name:
// known aliases first, then the same after stripping qualifiers such as
// "-inspired" or "(Nyonya)", then a case-insensitive match against
// KnownCuisines. An unknown cuisine keeps its own name, minus any qualifier
// ("Laotian-inspired" -> "Laotian").
func NormalizeCuisine(cuisine string) string {
	trimmed := strings.TrimSpace(cuisine)
	key := strings.ToLower(trimmed)
	stripped := false
	for {
		if canonical, ok := cuisineAliases[key]; ok {
			return canonical
		}
		for _, known := range KnownCuisines {
			if strings.EqualFold(key, known) {
				return known
			}
		}
		next := cuisineQualifier.ReplaceAllString(cuisineParenthetical.ReplaceAllString(key, ""), "")
		if next == key || next == "" {
			if stripped {
				return titleCase(key)
			}
			return trimmed
		}
		key, stripped = next, true
	}
}

// titleCase capitalises each word, treating hyphens and slashes as word
// breaks: "dutch-indonesian" -> "Dutch-Indonesian".
func titleCase(s string) string {
	b := []byte(s)
	for i := range b {
		if (i == 0 || strings.ContainsRune(" -/", rune(b[i-1]))) && b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 'a' - 'A'
		}
	}
	return string(b)
}

// AllCuisines returns a sorted, deduplicated list of every cuisine the app
// recognises: the hardcoded KnownCuisines plus any cuisine already present in
// the database (via counts). This is used to constrain the LLM's cuisine_type
// output so it never invents a new label that diverges from the existing set.
func AllCuisines(counts map[string]int) []string {
	seen := make(map[string]bool, len(KnownCuisines)+len(counts))
	all := make([]string, 0, len(KnownCuisines)+len(counts))
	for _, c := range KnownCuisines {
		if !seen[c] {
			seen[c] = true
			all = append(all, c)
		}
	}
	for c := range counts {
		if c != "" && !seen[c] {
			seen[c] = true
			all = append(all, c)
		}
	}
	slices.Sort(all)
	return all
}

// SeedCuisineCounts returns a copy of counts that includes every KnownCuisine
// at zero if it was not already present, so balancing logic sees the full list.
func SeedCuisineCounts(counts map[string]int) map[string]int {
	seeded := make(map[string]int, len(KnownCuisines))
	for _, c := range KnownCuisines {
		seeded[c] = 0
	}
	for c, n := range counts {
		seeded[c] = n
	}
	return seeded
}

// PickLeastRepresentedCuisine returns the cuisine with the fewest recipes.
// Alphabetical order is used as a tiebreaker for stable, deterministic picks.
// Returns an empty string if counts is empty.
func PickLeastRepresentedCuisine(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	return slices.MinFunc(slices.Collect(maps.Keys(counts)), func(a, b string) int {
		return cmp.Or(cmp.Compare(counts[a], counts[b]), strings.Compare(a, b))
	})
}

// systemPrompt holds the recipe-writing rules shared by every recipe call.
// It deliberately does not say "dinner": imports and refinements may be any
// kind of dish. Generation prompts add the dinner requirement themselves.
const systemPrompt = `You are an experienced recipe developer writing for home cooks. Your recipes are authentic to their cuisine, realistic to cook in a home kitchen, and precise enough to follow without guessing.

Respond with one recipe as a single JSON object with exactly these fields:
{
  "title": "Chicken Tikka Masala",
  "description": "Charred yogurt-marinated chicken in a creamy, gently spiced tomato sauce, served with basmati rice and a cucumber kachumber.",
  "cuisine_type": "Indian",
  "prep_time_minutes": 20,
  "cook_time_minutes": 35,
  "servings": 4,
  "difficulty": "medium",
  "ingredients": [
    {"name": "chicken thigh", "amount": 600, "unit": "g", "notes": "boneless, cut into chunks"},
    {"name": "onion", "amount": 1, "unit": ""},
    {"name": "garlic", "amount": 3, "unit": "clove"},
    {"name": "garam masala", "amount": 2, "unit": "tsp"},
    {"name": "basmati rice", "amount": 300, "unit": "g"},
    {"name": "cucumber", "amount": 1, "unit": "", "notes": "diced, for the kachumber"}
  ],
  "instructions": ["Mix the yogurt, ...", "..."],
  "dietary_restrictions": ["gluten-free"],
  "tags": ["high-protein"]
}

Writing rules:
- title: the dish's common name, not a marketing phrase.
- description: one sentence on what makes the dish appealing.
- ingredients: every ingredient the instructions use, including cooking oil, salt and water. Amounts must match "servings".
- instructions: 4-10 steps in cooking order, one or two sentences each, without step numbers. Give heat levels, times and doneness cues ("until golden, about 5 minutes").
- prep_time_minutes and cook_time_minutes: realistic estimates for a home cook.
- difficulty: "easy", "medium" or "hard".
- dietary_restrictions: only labels that truly apply, from: vegetarian, vegan, gluten-free, dairy-free, keto, paleo. Use [] if none apply.
- tags: 1-3 that truly apply, from: high-protein, low-carb, omega-3, low-calorie, high-fiber, meal-prep, quick, budget-friendly, one-pot, freezer-friendly.

Units:
- Metric only: g, kg, ml, l. Use tsp or tbsp for spices and small amounts of liquid.
- Whole items (onion, egg, tomato, bell pepper, lemon) use unit "" with a count. Garlic uses "clove".
- "amount" is always a single number (use 0.5, not "1/2" or "1-2").
- For seasoning to taste, give a sensible starting amount and put "to taste" in notes.

Respond with the JSON object only: no markdown, no code fences, no commentary.`

// SystemPrompt returns the recipe-writing system prompt without tool guidance.
// Used for single-shot calls (refine) where no tools are offered.
func SystemPrompt() string {
	return systemPrompt
}

// GenerationSystemPrompt returns the system prompt for tool-assisted recipe
// generation. Tool guidance only mentions tools that are actually offered, so
// the model is never told to call a tool it does not have.
func GenerationSystemPrompt(tools []Tool) string {
	var sb strings.Builder
	sb.WriteString(systemPrompt)

	has := func(name string) bool {
		return slices.ContainsFunc(tools, func(t Tool) bool { return t.Function.Name == name })
	}
	var steps []string
	if has("db_search") {
		steps = append(steps, "call db_search once with a short query for the dish you plan to make; if the library already has something similar, choose a clearly different dish")
	}
	if has("edamam_search") {
		steps = append(steps, "call edamam_search once to find an authentic reference recipe to adapt")
	}
	if len(steps) > 0 {
		sb.WriteString("\n\nBefore writing the recipe: ")
		sb.WriteString(strings.Join(steps, ", then "))
		sb.WriteString(".")
	}
	if has("web_search") {
		sb.WriteString(" Use web_search only to check a specific fact you are unsure of, such as a traditional technique or ratio.")
	}
	if len(steps) > 0 || has("web_search") {
		sb.WriteString(" After your research, respond with the recipe JSON.")
	}

	return sb.String()
}

// AvoidTitlesInstruction returns a suffix for the user turn listing existing
// recipes the model must not reproduce. It lives in the user turn because
// gemma4 regenerated listed dishes when the list was in the system prompt.
func AvoidTitlesInstruction(avoidTitles []string) string {
	if len(avoidTitles) == 0 {
		return ""
	}
	if len(avoidTitles) > 100 {
		avoidTitles = avoidTitles[:100]
	}
	return "\n\nThe library already has these dishes, so choose a different dish: not any of these, and not the same dish under another name. " + strings.Join(avoidTitles, "; ") + "."
}

// servingsRequirement is shared by generation prompts: small models often
// default to 4 servings unless told to scale explicitly.
func servingsRequirement(servings int) string {
	return fmt.Sprintf("Exactly %d servings: set \"servings\" to %d and scale every amount to match.", servings, servings)
}

// mealRules steer new recipes toward a whole dinner rather than a lone dish.
// Without them models write just the main, leaving the user to work out sides
// while ordering groceries; asking for the cuisine's own accompaniments keeps
// the result authentic instead of bolting generic rice and broccoli onto
// everything. They go in the user turn of new-recipe prompts only, where gemma4
// follows them most reliably; imports, refinements and leftover recipes share
// the system prompt and must not grow sides.
const mealRules = `Meal rules:
- The recipe is a whole dinner as it is served in its cuisine, not a lone dish. The ingredients and steps cover everything on the table, so the shopping list is complete.
- Every dinner has a protein (meat, fish, eggs, tofu, legumes or cheese), a starch (rice, potatoes, bread, pasta, noodles, grains or flatbread) and at least 125 g of vegetables per serving. Onion, garlic, herbs and chili do not count toward the vegetables.
- Get there the way the cuisine does. A dish that is already a complete plate (pho, bibimbap, paella, a hearty stew with potatoes) needs nothing extra. A main that is eaten with sides gets its traditional ones: steamed rice and a stir-fried green for a Chinese braise, rice and a sambal-dressed vegetable for a Malaysian curry, potatoes and a seasonal vegetable for a Dutch or German main, warm tortillas and a salsa or slaw for Mexican, flatbread and a salad for a Levantine grill.
- Keep sides simple: at most two, with only one starch on the plate (pasta needs no bread, potatoes need no pita). Use few extra ingredients, cook them alongside the main and write them as steps in cooking order.
- description: one sentence on what makes the dish appealing that names what it is served with.
- title: only the main dish's common name, without the sides ("Salmon Teriyaki", not "Salmon Teriyaki with Rice and Bok Choy"). Never add serving sizes or filler such as "for One", "for Two", "Easy" or "Ultimate".`

// BuildGeneratePrompt builds the user prompt for a single generation.
// Tool guidance and avoid-titles live in the system prompt.
func BuildGeneratePrompt(req models.GenerateRequest, cuisineCounts map[string]int) string {
	cuisineType := req.CuisineType
	if cuisineType == "" {
		cuisineType = PickLeastRepresentedCuisine(SeedCuisineCounts(cuisineCounts))
	}

	var sb strings.Builder
	sb.WriteString("Create a dinner recipe")
	if cuisineType != "" {
		fmt.Fprintf(&sb, " from %s cuisine", cuisineType)
	}
	sb.WriteString(".")

	sb.WriteString("\n\n" + mealRules)

	var reqs []string
	if req.Servings > 0 {
		reqs = append(reqs, servingsRequirement(req.Servings))
	}
	if len(req.DietaryRestrictions) > 0 {
		reqs = append(reqs, fmt.Sprintf("Must be %s; include these in dietary_restrictions.", strings.Join(req.DietaryRestrictions, " and ")))
	}
	if req.MaxPrepTime > 0 {
		reqs = append(reqs, fmt.Sprintf("Prep time at most %d minutes.", req.MaxPrepTime))
	}
	if req.Difficulty != "" {
		reqs = append(reqs, fmt.Sprintf("Difficulty: %s.", req.Difficulty))
	}
	if notes := strings.TrimSpace(req.AdditionalNotes); notes != "" {
		reqs = append(reqs, "The user also asked for: "+notes)
	}
	if cuisines := AllCuisines(cuisineCounts); len(cuisines) > 0 {
		reqs = append(reqs, "cuisine_type must be one of: "+strings.Join(cuisines, ", ")+".")
	}
	if len(reqs) > 0 {
		sb.WriteString("\n\nRequirements:\n- ")
		sb.WriteString(strings.Join(reqs, "\n- "))
	}
	return sb.String()
}

// BuildBackgroundGeneratePrompt creates the user prompt for unattended background
// generation. targetCuisine may be empty to let the model choose; allCuisines
// constrains cuisine_type when non-empty.
func BuildBackgroundGeneratePrompt(targetCuisine string, allCuisines []string, index, total, servings int) string {
	var sb strings.Builder
	if targetCuisine != "" {
		fmt.Fprintf(&sb, "Create a %s dinner recipe.", targetCuisine)
	} else {
		sb.WriteString("Create a dinner recipe.")
	}
	sb.WriteString("\n\n" + mealRules)
	sb.WriteString("\n\nRequirements:\n- " + servingsRequirement(servings))
	if len(allCuisines) > 0 {
		sb.WriteString("\n- cuisine_type must be one of: " + strings.Join(allCuisines, ", ") + ".")
	}
	if total > 1 {
		fmt.Fprintf(&sb, "\n- This is recipe %d of %d in a batch; make it clearly different from the others.", index, total)
	}
	return sb.String()
}

// BuildRefinePrompt asks for a revised recipe that applies only the user's
// requested change. The recipe is passed as JSON in the same shape the model
// must answer in, so nothing is lost converting between formats.
func BuildRefinePrompt(recipe models.Recipe, feedback string) string {
	current := struct {
		Title               string              `json:"title"`
		Description         string              `json:"description"`
		CuisineType         string              `json:"cuisine_type"`
		PrepTimeMinutes     int                 `json:"prep_time_minutes"`
		CookTimeMinutes     int                 `json:"cook_time_minutes"`
		Servings            int                 `json:"servings"`
		Difficulty          string              `json:"difficulty"`
		Ingredients         []models.Ingredient `json:"ingredients"`
		Instructions        []string            `json:"instructions"`
		DietaryRestrictions []string            `json:"dietary_restrictions"`
		Tags                []string            `json:"tags"`
	}{
		recipe.Title, recipe.Description, recipe.CuisineType, recipe.PrepTimeMinutes, recipe.CookTimeMinutes,
		recipe.Servings, recipe.Difficulty, recipe.Ingredients, recipe.Instructions, recipe.DietaryRestrictions, recipe.Tags,
	}
	data, _ := json.MarshalIndent(current, "", "  ")

	return fmt.Sprintf(`Revise this recipe:

%s

Requested change: %s

Apply the requested change and everything it implies: swapping an ingredient also updates the steps that use it, and changing servings rescales every amount. Keep everything else the same, including the title, servings and cuisine, unless the change asks otherwise. Respond with the complete revised recipe JSON.`, data, strings.TrimSpace(feedback))
}

// BuildLeftoverPrompt builds the user prompt for a leftover-ingredient recipe.
func BuildLeftoverPrompt(ingredients []string) string {
	return fmt.Sprintf(`Create a recipe that uses up these leftover ingredients: %s

Use as many of them as possible, prioritising the most perishable. You may add common pantry staples (oil, butter, salt, pepper, dried spices, onion, garlic, stock, flour, rice or pasta), but avoid other ingredients that would need a shopping trip.`, strings.Join(ingredients, ", "))
}

// BuildImportPrompt builds the user prompt for importing free-form recipe text.
func BuildImportPrompt(rawText string) string {
	return fmt.Sprintf(`Turn the text below into a structured recipe.

- If it contains ingredients or steps, keep them faithfully: same ingredients, same method. Only convert units to metric, fix obvious typos, and split or merge steps for clarity.
- Fill in anything missing (amounts, times, servings, description) with sensible values.
- If it is only a dish name, write the most traditional version of that dish.
- Keep the dish's original title.

Text:
"""
%s
"""`, strings.TrimSpace(rawText))
}

func BuildScanIngredientPrompt() string {
	return `Identify the food products shown in this image.

Name each PRODUCT (e.g. "soy sauce", "whole milk", "canned tomatoes"), not the sub-ingredients printed on its label. Use a visible label only to confirm the product name and its net weight or volume.

Respond with JSON in this shape, one item per distinct product:
{"items": [{"name": "whole milk", "amount": 1, "unit": "l", "confident": true}]}

- name: the most specific product name you can determine, in English, lowercase.
- amount: net weight or volume printed on the package, otherwise 0.
- unit: unit of the amount (g, kg, ml, l), or "" if unknown.
- confident: true only if you are sure of the name and, when amount > 0, of the amount.`
}

// BuildDeduplicateIngredientsPrompt asks the LLM to merge groups of ingredients
// that share a name but couldn't be merged deterministically, usually because
// they mix weight and volume for an ingredient without a known density.
func BuildDeduplicateIngredientsPrompt(groupsJSON string, groupCount int) string {
	return fmt.Sprintf(`You are consolidating a grocery shopping list. Each group below lists the same item in different units that could not be merged automatically.

Groups:
%s

Return exactly one item per group, in the same order as the groups. There are %d groups, so return %d items.

For each item:
- name: generic English name, lowercase.
- calculation: convert EVERY entry in the group to one unit using a typical density or weight for that ingredient, and add them all up.
- amount and unit: the total from the calculation. Prefer the unit you would buy in: g or kg for solids, ml or l for liquids, "" for whole items counted individually. Round whole items (eggs, onions, lemons) up to a whole number.

Example: the group [{"name":"Parmesan","amount":100,"unit":"g"},{"name":"parmesan","amount":1,"unit":"cup"}] becomes
{"name": "parmesan", "calculation": "100 g + 1 cup grated (~100 g) = 200 g", "amount": 200, "unit": "g"}`, groupsJSON, groupCount, groupCount)
}
