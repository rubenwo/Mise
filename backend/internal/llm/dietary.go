package llm

import (
	"log"
	"slices"
	"strings"

	"github.com/rubenwo/mise/internal/models"
)

// Models often attach dietary labels their own ingredients contradict (a
// "gluten-free" tagine served with couscous, a "vegetarian" dish with fish
// sauce). These word lists catch the common cases; an ingredient whose name
// contains one of the safe phrases is left alone.

var meatWords = []string{
	"chicken", "beef", "pork", "lamb", "mutton", "veal", "duck", "turkey", "bacon", "ham",
	"sausage", "chorizo", "salami", "prosciutto", "pancetta", "mince", "steak", "rookworst",
	"fish", "salmon", "tuna", "cod", "haddock", "halibut", "tilapia", "trout", "mackerel",
	"sardine", "anchovy", "anchovies", "shrimp", "prawn", "crab", "lobster", "squid", "sotong",
	"octopus", "mussel", "clam", "scallop", "oyster", "belacan", "bonito", "dashi", "gelatin",
	"gelatine", "lard", "worcestershire",
}

// Goat is meat except as goat's cheese or milk.
var goatMeat = []string{"goat meat", "goat leg", "goat shoulder", "goat chop", "goat stew", "goat curry"}

var dairyWords = []string{
	"milk", "butter", "cheese", "cream", "yogurt", "yoghurt", "ghee", "paneer", "feta", "parmesan",
	"mozzarella", "ricotta", "halloumi", "kefir", "buttermilk", "creme", "crème", "kibbeh",
	"mascarpone", "gouda", "cheddar", "emmental", "gruyere", "gruyère", "brie", "burrata",
}

var veganOnlyWords = []string{"egg", "eggs", "honey", "mayonnaise", "mayo"}

var glutenWords = []string{
	"wheat", "flour", "couscous", "bulgur", "bulghur", "semolina", "pasta", "spaghetti", "penne",
	"rigatoni", "fusilli", "linguine", "tagliatelle", "lasagna", "lasagne", "macaroni", "orzo",
	"noodle", "noodles", "udon", "ramen", "bread", "breadcrumbs", "panko", "tortilla", "pita",
	"naan", "roti", "baguette", "ciabatta", "brioche", "barley", "rye", "spelt", "farro", "seitan",
	"soy sauce", "beer", "malt", "crouton", "phyllo", "filo", "pastry", "dumpling", "wonton",
	"cracker", "gnocchi", "spätzle", "spaetzle", "biscuit", "kecap",
}

// Phrases that make an otherwise matching ingredient fine for the label.
var plantSafe = []string{"vegan", "vegetarian", "plant", "meatless", "mushroom", "jackfruit", "tofu",
	"substitute", "alternative", "butter lettuce", "butter bean",
	"peanut butter", "almond butter", "cashew butter", "nut butter", "seed butter", "coconut",
	"oat milk", "soy milk", "almond milk", "rice milk", "cacao butter", "cocoa butter", "eggplant"}

var glutenSafe = []string{"gluten-free", "gluten free", "rice", "chickpea", "corn", "maize", "buckwheat",
	"teff", "tapioca", "potato", "almond flour", "coconut flour", "tamari", "glass noodle",
	"sweet potato noodle", "cassava", "sorghum", "millet", "quinoa",
	"vermicelli", // in these recipes nearly always rice vermicelli
}

func containsWord(name, word string) bool {
	if strings.Contains(word, " ") {
		return strings.Contains(name, word)
	}
	for _, w := range strings.FieldsFunc(name, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 0xC0) // letters, keeping accented ones
	}) {
		if w == word || w == word+"s" || w == word+"es" {
			return true
		}
	}
	return false
}

func matchesAny(name string, words []string) bool {
	return slices.ContainsFunc(words, func(w string) bool { return containsWord(name, w) })
}

func hasPhrase(name string, phrases []string) bool {
	return slices.ContainsFunc(phrases, func(p string) bool { return strings.Contains(name, p) })
}

func isMeat(name string) bool {
	if hasPhrase(name, plantSafe) {
		return false
	}
	return matchesAny(name, meatWords) || hasPhrase(name, goatMeat) ||
		(containsWord(name, "goat") && !strings.Contains(name, "cheese") && !strings.Contains(name, "milk"))
}

func isDairy(name string) bool {
	return !hasPhrase(name, plantSafe) && matchesAny(name, dairyWords)
}

func isGluten(name string) bool {
	return !hasPhrase(name, glutenSafe) && matchesAny(name, glutenWords)
}

// DropContradictedDiets removes dietary labels that an ingredient
// contradicts and returns the labels it removed. Generation runs it on every
// new recipe; a startup sweep applies it to the existing library.
func DropContradictedDiets(recipe *models.Recipe) []string {
	conflict := map[string]string{} // label -> first offending ingredient
	for _, ing := range recipe.Ingredients {
		// "chicken or vegetable stock": a label only conflicts when every
		// alternative does.
		options := strings.Split(strings.ToLower(ing.Name), " or ")
		check := func(label string, bad func(string) bool) {
			if conflict[label] == "" && !slices.ContainsFunc(options, func(o string) bool { return !bad(o) }) {
				conflict[label] = ing.Name
			}
		}
		check("vegetarian", isMeat)
		check("vegan", func(n string) bool {
			return isMeat(n) || isDairy(n) || (!hasPhrase(n, plantSafe) && matchesAny(n, veganOnlyWords))
		})
		check("dairy-free", isDairy)
		check("gluten-free", isGluten)
	}
	var removed []string
	recipe.DietaryRestrictions = slices.DeleteFunc(recipe.DietaryRestrictions, func(d string) bool {
		if ing, ok := conflict[strings.ToLower(d)]; ok {
			log.Printf("Review: removed %q label from %q: contains %q", d, recipe.Title, ing)
			removed = append(removed, d)
			return true
		}
		return false
	})
	return removed
}
