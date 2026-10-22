// Package ingredients normalizes, converts, and merges recipe ingredients
// into a consolidated shopping list.
package ingredients

import (
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/rubenwo/mise/internal/models"
	"github.com/rubenwo/mise/internal/textsim"
)

// sizeAdjectives are leading qualifiers that do not change what ingredient to buy.
// These are stripped from the front of an ingredient name (repeatedly) during normalization.
// Only strip from the front to avoid breaking compound ingredients like "ground beef".
var sizeAdjectives = map[string]bool{
	// Size / quantity
	"large": true, "small": true, "medium": true, "whole": true, "extra": true,
	// State
	"fresh": true, "freshly": true, "raw": true, "frozen": true, "dried": true,
	"cooked": true, "roasted": true, "toasted": true,
	// Prep method (leading only)
	"minced": true, "chopped": true, "diced": true, "sliced": true,
	"grated": true, "shredded": true, "crushed": true, "peeled": true,
	"rinsed": true, "softened": true, "melted": true, "sifted": true, "packed": true,
	// Animal cut qualifiers
	"tender": true, "boneless": true, "skinless": true, "lean": true, "trimmed": true,
	// Quality / origin
	"organic": true, "natural": true,
}

// ingredientAliases maps a fully-normalized name to its canonical equivalent.
// Applied after all other normalization so that stripping can produce matchable keys.
// Keys and values must be lowercase and already free of parentheticals/commas.
// NOTE: keys are written in their POST-normalization form (leading sizeAdjectives already
// stripped, trailing 's' already removed, parentheticals removed). For example:
//   "whole wheat flour" is never hit because "whole" is stripped → key must be "wheat flour".
//   "garlic cloves" is never hit because trailing 's' stripped → key must be "garlic clove".
var ingredientAliases = map[string]string{

	// ── Flour ──────────────────────────────────────────────────────────────────
	// "whole wheat flour" → "whole" stripped by sizeAdjectives → key is "wheat flour"
	"all purpose flour":  "flour",
	"all-purpose flour":  "flour",
	"wheat flour":        "flour",
	"plain flour":        "flour", // UK all-purpose
	"self rising flour":  "flour",
	"self-rising flour":  "flour",
	"self raising flour": "flour",
	"self-raising flour": "flour",
	"bread flour":        "flour",
	"cake flour":         "flour",
	"rice flour":         "flour",
	"00 flour":           "flour",
	"tipo 00":            "flour",
	// almond flour, spelt flour, buckwheat flour kept distinct — specialty items

	// ── Sugar ──────────────────────────────────────────────────────────────────
	"granulated sugar":     "sugar",
	"white sugar":          "sugar",
	"caster sugar":         "sugar", // UK superfine
	"castor sugar":         "sugar", // alternate spelling
	"superfine sugar":      "sugar",
	"cane sugar":           "sugar",
	"golden caster sugar":  "sugar",
	"demerara sugar":       "brown sugar",
	"muscovado":            "brown sugar",
	"muscovado sugar":      "brown sugar",
	"dark brown sugar":     "brown sugar",
	"light brown sugar":    "brown sugar",
	"powdered sugar":       "icing sugar", // US → UK canonical
	"confectioners sugar":  "icing sugar",
	"confectioners' sugar": "icing sugar",
	"treacle":              "molasses", // UK dark syrup → US equivalent
	"dark treacle":         "molasses",

	// ── Oil ────────────────────────────────────────────────────────────────────
	// "extra virgin olive oil" → "extra" stripped → key is "virgin olive oil"
	"virgin olive oil": "olive oil",
	"canola oil":       "vegetable oil", // North American name for rapeseed
	"rapeseed oil":     "vegetable oil", // UK/EU name for canola
	"corn oil":         "vegetable oil",
	"groundnut oil":    "peanut oil", // UK term

	// ── Pepper ─────────────────────────────────────────────────────────────────
	"black pepper":         "pepper",
	"ground black pepper":  "pepper",
	"cracked black pepper": "pepper",
	"black peppercorn":     "pepper",
	"ground white pepper":  "white pepper",
	"white peppercorn":     "white pepper",

	// ── Stock / Broth ───────────────────────────────────────────────────────────
	"chicken broth":   "chicken stock",
	"vegetable broth": "vegetable stock",
	"beef broth":      "beef stock",
	"fish broth":      "fish stock",
	"pork broth":      "pork stock",
	"lamb broth":      "lamb stock",

	// ── Cream ──────────────────────────────────────────────────────────────────
	"heavy cream":          "cream",
	"heavy whipping cream": "cream",
	"double cream":         "cream", // UK full-fat
	"whipping cream":       "cream",
	"single cream":         "light cream", // UK ~18% fat
	"half and half":        "light cream", // US ~10-12% fat

	// ── Milk ───────────────────────────────────────────────────────────────────
	// "whole milk" → "whole" stripped → "milk"; no alias needed
	"full fat milk":  "milk",
	"full-fat milk":  "milk",
	"skimmed milk":   "skim milk", // UK spelling
	"nonfat milk":    "skim milk", // US alternate
	"non-fat milk":   "skim milk",

	// ── Yogurt ─────────────────────────────────────────────────────────────────
	// "natural yogurt/yoghurt" → "natural" stripped → "yogurt"/"yoghurt"; only need yoghurt alias
	"yoghurt":       "yogurt",       // UK spelling
	"plain yogurt":  "yogurt",
	"plain yoghurt": "yogurt",
	"greek yoghurt": "greek yogurt", // UK spelling

	// ── Butter ─────────────────────────────────────────────────────────────────
	"unsalted butter": "butter",
	"salted butter":   "butter",

	// ── Garlic ─────────────────────────────────────────────────────────────────
	"garlic clove": "garlic",

	// ── Onion family ───────────────────────────────────────────────────────────
	"spring onion":  "green onion", // UK → US
	"scallion":      "green onion", // US regional → canonical
	"eschallot":     "shallot",     // Australian English
	"green shallot": "green onion", // Australian English

	// ── Tomato ─────────────────────────────────────────────────────────────────
	"tomato puree":       "tomato paste",
	"tomato concentrate": "tomato paste",
	"tinned tomato":      "canned tomato", // UK "tin" = US "can"

	// ── Vegetables — regional / alternate names ─────────────────────────────────
	"aubergine":     "eggplant",       // UK/FR → US
	"courgette":     "zucchini",       // UK/FR → US/IT
	"rocket":        "arugula",        // UK/AU → US/IT
	"swede":         "rutabaga",       // UK → US
	"beetroot":      "beet",           // UK → US
	"mangetout":     "snow pea",       // UK/FR → US
	"mange tout":    "snow pea",
	"capsicum":      "bell pepper",    // AU/NZ → US
	"cos lettuce":   "romaine lettuce",
	"romaine":       "romaine lettuce",
	"celery root":   "celeriac",       // US alternate
	"pak choi":      "bok choy",       // UK spelling
	"pak choy":      "bok choy",
	"baby corn":     "corn",
	"sweetcorn":     "corn",           // UK → US
	"corn on the cob": "corn",

	// ── Herbs ──────────────────────────────────────────────────────────────────
	// "coriander" alone safely maps to cilantro: compound forms like
	// "coriander seed" and "ground coriander" are never affected by this alias.
	"coriander":      "cilantro", // UK/EU leaf name → US name
	"coriander leaf": "cilantro",
	"coriander leave": "cilantro", // post-normalization form of "coriander leaves"
	// Parsley variants
	"flat leaf parsley":  "parsley",
	"flat-leaf parsley":  "parsley",
	"curly parsley":      "parsley",
	"italian parsley":    "parsley",

	// ── Cheese ─────────────────────────────────────────────────────────────────
	"parmigiano":          "parmesan",
	"parmigiano reggiano": "parmesan",
	"parmesan cheese":     "parmesan",
	"grana padano":        "parmesan", // similar hard Italian cheese; interchangeable for shopping
	"pecorino romano":     "pecorino",
	"mozzarella cheese":   "mozzarella",
	"feta cheese":         "feta",

	// ── Legumes ────────────────────────────────────────────────────────────────
	"garbanzo":            "chickpea", // US alternate → international canonical
	"garbanzo bean":       "chickpea",
	"haricot bean":        "navy bean",     // UK → US
	"cannellini bean":     "white bean",
	"great northern bean": "white bean",
	"butter bean":         "lima bean",     // UK → US
	"broad bean":          "fava bean",     // UK → US/IT
	"borlotti bean":       "cranberry bean",

	// ── Ground meat ────────────────────────────────────────────────────────────
	"beef mince":    "ground beef",    // UK → US
	"pork mince":    "ground pork",
	"turkey mince":  "ground turkey",
	"lamb mince":    "ground lamb",
	"chicken mince": "ground chicken",
	"mince beef":    "ground beef",

	// ── Seafood ────────────────────────────────────────────────────────────────
	"prawn":      "shrimp", // UK/AU → US
	"king prawn": "shrimp",

	// ── Pork cuts ──────────────────────────────────────────────────────────────
	"pork butt":   "pork shoulder",
	"boston butt": "pork shoulder",
	"pork collar": "pork shoulder",
	"pork neck":   "pork shoulder",

	// ── Pantry staples ──────────────────────────────────────────────────────────
	"cornflour":           "cornstarch",    // UK name for cornstarch
	"corn flour":          "cornstarch",
	"bicarbonate of soda": "baking soda",   // UK → US
	"bicarb":              "baking soda",
	"bicarb soda":         "baking soda",   // AU term
	"soya sauce":          "soy sauce",     // UK spelling
	"shoyu":               "soy sauce",     // Japanese name
	"vanilla essence":     "vanilla extract", // UK term
	"balsamic":            "balsamic vinegar",
	"passata":             "tomato passata", // common name shorthand

	// ── Nuts ───────────────────────────────────────────────────────────────────
	"groundnut": "peanut",   // UK term
	"filbert":   "hazelnut", // old/regional name for hazelnut
}

// nonPluralNames end in "s" but are not plurals, so the trailing "s" is kept.
var nonPluralNames = map[string]bool{
	"molasses": true,
	"bitters":  true,
	"grits":    true,
}

// ingredientDensity maps a normalized ingredient name to its density in g/mL,
// used to convert between weight and volume for the same ingredient.
var ingredientDensity = map[string]float64{
	"flour":             0.53,
	"all-purpose flour": 0.53,
	"bread flour":       0.56,
	"cake flour":        0.44,
	"whole wheat flour": 0.52,
	"wheat flour":       0.52,
	"sugar":             0.845,
	"granulated sugar":  0.845,
	"brown sugar":       0.72,
	"powdered sugar":    0.56,
	"icing sugar":       0.56,
	"salt":              1.20,
	"butter":            0.91,
	"olive oil":         0.91,
	"oil":               0.91,
	"vegetable oil":     0.91,
	"sunflower oil":     0.91,
	"water":             1.00,
	"milk":              1.03,
	"cream":             0.97,
	"honey":             1.42,
	"rice":              0.75,
	"oat":               0.35,
	"rolled oat":        0.35,
	"cornstarch":        0.61,
	"baking powder":     0.90,
	"baking soda":       1.08,
	"cocoa powder":      0.48,
	"almond flour":      0.44,
}

// Normalize strips common leading adjectives, parentheticals,
// trailing modifiers and simple plurals so variant names key together.
// It then applies the ingredientAliases table to canonicalize known synonyms.
func Normalize(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	// Strip parenthetical notes: "butter (unsalted)" → "butter"
	if idx := strings.Index(name, "("); idx >= 0 {
		name = strings.TrimSpace(name[:idx])
	}
	// Strip after comma: "butter, softened" → "butter"
	if idx := strings.Index(name, ","); idx >= 0 {
		name = strings.TrimSpace(name[:idx])
	}
	// Strip common trailing phrases
	for _, suffix := range []string{" for serving", " for garnish", " for coating", " to taste", " to serve"} {
		name = strings.TrimSuffix(name, suffix)
	}
	// Strip leading size/quality/prep adjectives (only when words remain after stripping).
	// Repeated so "freshly ground black pepper" → "ground black pepper" → "black pepper".
	words := strings.Fields(name)
	for len(words) > 1 && sizeAdjectives[words[0]] {
		words = words[1:]
	}
	name = strings.Join(words, " ")
	// Normalize simple plurals: "eggs" → "egg", "onions" → "onion"
	// Skip "-ss"/"-us" words ("swiss", "hummus", "asparagus") and known
	// singular-looking exceptions like "molasses".
	if len(name) > 3 && strings.HasSuffix(name, "s") &&
		!strings.HasSuffix(name, "ss") && !strings.HasSuffix(name, "us") && !nonPluralNames[name] {
		name = name[:len(name)-1]
	}
	// Apply synonym/alias table for known equivalents (flour variants, broth↔stock, etc.)
	if canonical, ok := ingredientAliases[name]; ok {
		name = canonical
	}
	return name
}

// isCountableUnit returns true for units that represent indivisible whole items.
func isCountableUnit(unit string) bool {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "piece", "pieces", "whole", "count", "pcs", "pc",
		"clove", "cloves", "slice", "slices", "head", "heads",
		"stalk", "stalks", "sprig", "sprigs", "bunch", "bunches",
		"can", "cans", "sheet", "sheets":
		return true
	}
	return false
}

// unitToBase converts an amount+unit to a canonical base unit for aggregation.
// Base units: "g" for weight, "ml" for volume. Other units are returned as-is.
func unitToBase(amount float64, unit string) (float64, string) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "kg":
		return amount * 1000, "g"
	case "oz":
		return amount * 28.35, "g"
	case "lb", "lbs":
		return amount * 453.592, "g"
	case "l", "liter", "litre", "liters", "litres":
		return amount * 1000, "ml"
	case "tsp", "teaspoon", "teaspoons":
		return amount * 4.929, "ml"
	case "tbsp", "tablespoon", "tablespoons":
		return amount * 14.787, "ml"
	case "cup", "cups":
		return amount * 240, "ml"
	default:
		return amount, strings.ToLower(strings.TrimSpace(unit))
	}
}

// baseToDisplay converts a base-unit amount to a human-friendly unit.
func baseToDisplay(amount float64, baseUnit string) (float64, string) {
	switch baseUnit {
	case "g":
		if amount >= 1000 {
			return amount / 1000, "kg"
		}
		return amount, "g"
	case "ml":
		if amount >= 1000 {
			return amount / 1000, "l"
		}
		return amount, "ml"
	default:
		return amount, baseUnit
	}
}

// roundAmount rounds to a whole number for amounts >= 10, otherwise 2 decimal places.
func roundAmount(amount float64) float64 {
	if amount >= 10 {
		return math.Round(amount)
	}
	return math.Round(amount*100) / 100
}

// compactName removes spaces and hyphens so that "chicken breast"
// and "chickenbreast" produce the same compact key for fuzzy comparison.
func compactName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r != ' ' && r != '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// nameSimilarity returns a [0,1] similarity score between two
// already-normalized ingredient names. It strips spaces/hyphens before
// comparing so that "chicken breast" and "chickenbreast" score 1.0.
// Uses the max of edit-distance similarity and character-bigram Jaccard.
func nameSimilarity(a, b string) float64 {
	ca, cb := compactName(a), compactName(b)
	if ca == cb {
		return 1.0
	}
	maxLen := max(len(ca), len(cb))
	if maxLen == 0 {
		return 1.0
	}
	editSim := 1.0 - float64(textsim.Levenshtein(ca, cb))/float64(maxLen)
	return max(editSim, textsim.JaccardBigrams(ca, cb))
}

// preferredName returns the "better" of two fuzzy-equivalent names.
// Prefers more words (properly spaced), then longer, then lexicographically smaller.
func preferredName(a, b string) string {
	wa, wb := len(strings.Fields(a)), len(strings.Fields(b))
	if wa != wb {
		if wa > wb {
			return a
		}
		return b
	}
	if len(a) != len(b) {
		if len(a) > len(b) {
			return a
		}
		return b
	}
	if a < b {
		return a
	}
	return b
}

// canonicalNames clusters the given names by similarity and returns a
// map from each name to its group's canonical representative.
// Names whose compact forms are identical, or whose similarity exceeds the
// threshold, are merged into one group. The canonical is the "best" name
// as determined by preferredName.
func canonicalNames(names []string) map[string]string {
	const threshold = 0.85

	// Union-Find: parent[n] starts as n itself.
	parent := make(map[string]string, len(names))
	for _, n := range names {
		parent[n] = n
	}
	var find func(string) string
	find = func(n string) string {
		if parent[n] != n {
			parent[n] = find(parent[n]) // path compression
		}
		return parent[n]
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		// Pick the preferred name as the new root.
		if preferredName(ra, rb) == ra {
			parent[rb] = ra
		} else {
			parent[ra] = rb
		}
	}

	// O(n²) similarity check — fine for typical ingredient counts (~50-150).
	for i, a := range names {
		for _, b := range names[i+1:] {
			if nameSimilarity(a, b) >= threshold {
				union(a, b)
			}
		}
	}

	// Build final canonical map with full path compression.
	result := make(map[string]string, len(names))
	for _, n := range names {
		result[n] = find(n)
	}
	return result
}

// Consolidate merges duplicate ingredients by:
//  1. normalizing names and converting to base units
//  2. fuzzy-grouping similar names (handles typos and compound-word variants)
//  3. re-aggregating by {canonicalName, baseUnit}
//  4. cross-unit merging g↔ml using known ingredient densities
//  5. rounding countable items up to the nearest whole number
func Consolidate(ingredients []models.AggregatedIngredient) []models.AggregatedIngredient {
	type aggKey struct{ name, unit string }
	type entry struct {
		normName string
		amount   float64
		baseUnit string
		recipes  map[string]bool
	}

	// Pass 1: normalize + base-unit convert, aggregate by {normName, baseUnit}.
	agg := map[aggKey]*entry{}
	for _, ing := range ingredients {
		normName := Normalize(ing.Name)
		baseAmount, baseUnit := unitToBase(ing.Amount, ing.Unit)
		k := aggKey{normName, baseUnit}
		if agg[k] == nil {
			agg[k] = &entry{normName: normName, baseUnit: baseUnit, recipes: map[string]bool{}}
		}
		agg[k].amount += baseAmount
		for _, r := range ing.Recipes {
			agg[k].recipes[r] = true
		}
	}

	// Fuzzy name grouping: collect distinct normNames, cluster by similarity,
	// then re-key the agg map so that fuzzy-equivalent names share one entry.
	{
		nameSet := make(map[string]struct{}, len(agg))
		for k := range agg {
			nameSet[k.name] = struct{}{}
		}
		names := make([]string, 0, len(nameSet))
		for n := range nameSet {
			names = append(names, n)
		}
		canon := canonicalNames(names)

		// Only re-aggregate when at least one name maps to a different canonical.
		needsRemap := false
		for n, c := range canon {
			if n != c {
				needsRemap = true
				break
			}
		}
		if needsRemap {
			type aggKey2 = aggKey
			agg2 := map[aggKey2]*entry{}
			for k, e := range agg {
				c := canon[k.name]
				k2 := aggKey2{c, k.unit}
				if agg2[k2] == nil {
					agg2[k2] = &entry{normName: c, baseUnit: k.unit, recipes: map[string]bool{}}
				}
				agg2[k2].amount += e.amount
				for r := range e.recipes {
					agg2[k2].recipes[r] = true
				}
			}
			agg = agg2
		}
	}

	// Pass 2: group by normName; attempt g↔ml cross-unit merge via density table.
	byName := map[string][]*entry{}
	for _, e := range agg {
		byName[e.normName] = append(byName[e.normName], e)
	}

	var merged []*entry
	for _, group := range byName {
		if len(group) == 1 {
			merged = append(merged, group[0])
			continue
		}
		// Separate into g, ml, and other-unit buckets.
		var gAmt, mlAmt float64
		var gRecipes, mlRecipes []string
		hasG, hasML := false, false
		var others []*entry
		for _, e := range group {
			switch e.baseUnit {
			case "g":
				gAmt += e.amount
				for r := range e.recipes {
					gRecipes = append(gRecipes, r)
				}
				hasG = true
			case "ml":
				mlAmt += e.amount
				for r := range e.recipes {
					mlRecipes = append(mlRecipes, r)
				}
				hasML = true
			default:
				others = append(others, e)
			}
		}
		if hasG && hasML {
			density, known := ingredientDensity[group[0].normName]
			if known {
				// Convert ml → g and merge into single g entry.
				combined := &entry{
					normName: group[0].normName,
					baseUnit: "g",
					amount:   gAmt + mlAmt*density,
					recipes:  map[string]bool{},
				}
				for _, r := range gRecipes {
					combined.recipes[r] = true
				}
				for _, r := range mlRecipes {
					combined.recipes[r] = true
				}
				merged = append(merged, combined)
				merged = append(merged, others...)
				continue
			}
		}
		// Cannot merge — leave all entries separate (LLM fallback will handle them).
		merged = append(merged, group...)
	}

	// Pass 3: build output with display units and countable ceiling.
	result := make([]models.AggregatedIngredient, 0, len(merged))
	for _, e := range merged {
		amount, unit := baseToDisplay(e.amount, e.baseUnit)
		if isCountableUnit(unit) {
			amount = math.Ceil(amount)
		} else {
			amount = roundAmount(amount)
		}
		displayName := e.normName
		if len(displayName) > 0 {
			displayName = strings.ToUpper(displayName[:1]) + displayName[1:]
		}
		result = append(result, models.AggregatedIngredient{
			Name:    displayName,
			Amount:  amount,
			Unit:    unit,
			Recipes: slices.Sorted(maps.Keys(e.recipes)),
		})
	}
	sortByName(result)
	return result
}

func sortByName(ingredients []models.AggregatedIngredient) {
	slices.SortFunc(ingredients, func(a, b models.AggregatedIngredient) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}

// FindRemainingDuplicates returns groups (≥2 items) that still share the same
// normalized name after deterministic consolidation.
func FindRemainingDuplicates(ingredients []models.AggregatedIngredient) [][]models.AggregatedIngredient {
	groups := map[string][]int{}
	for i, ing := range ingredients {
		groups[Normalize(ing.Name)] = append(groups[Normalize(ing.Name)], i)
	}
	var result [][]models.AggregatedIngredient
	for _, indices := range groups {
		if len(indices) < 2 {
			continue
		}
		group := make([]models.AggregatedIngredient, len(indices))
		for j, idx := range indices {
			group[j] = ingredients[idx]
		}
		result = append(result, group)
	}
	return result
}

// ApplyMerged replaces the duplicate groups in ingredients with the LLM-merged items.
func ApplyMerged(ingredients []models.AggregatedIngredient, dupes [][]models.AggregatedIngredient, merged []models.AggregatedIngredient) []models.AggregatedIngredient {
	remove := map[string]bool{}
	for _, group := range dupes {
		for _, ing := range group {
			remove[Normalize(ing.Name)] = true
		}
	}
	result := make([]models.AggregatedIngredient, 0, len(ingredients))
	for _, ing := range ingredients {
		if !remove[Normalize(ing.Name)] {
			result = append(result, ing)
		}
	}
	result = append(result, merged...)
	sortByName(result)
	return result
}

// Aggregate sums the ingredients of every recipe in a plan, scaled to the
// planned servings, and consolidates the result into a shopping list.
func Aggregate(plan *models.MealPlan) []models.AggregatedIngredient {
	type key struct{ name, unit string }
	agg := map[key]*models.AggregatedIngredient{}
	recipeNames := map[key]map[string]bool{}

	for _, mpr := range plan.Recipes {
		scale := 1.0
		if mpr.Recipe.Servings > 0 {
			scale = float64(mpr.Servings) / float64(mpr.Recipe.Servings)
		}
		for _, ing := range mpr.Recipe.Ingredients {
			k := key{Normalize(ing.Name), strings.ToLower(ing.Unit)}
			if agg[k] == nil {
				agg[k] = &models.AggregatedIngredient{Name: ing.Name, Unit: ing.Unit}
				recipeNames[k] = map[string]bool{}
			}
			agg[k].Amount += ing.Amount * scale
			recipeNames[k][mpr.Recipe.Title] = true
		}
	}

	result := make([]models.AggregatedIngredient, 0, len(agg))
	for k, v := range agg {
		v.Amount = math.Round(v.Amount*100) / 100
		v.Recipes = slices.Sorted(maps.Keys(recipeNames[k]))
		result = append(result, *v)
	}
	return Consolidate(result)
}
