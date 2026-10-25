// Package recommend suggests recipes the user hasn't cooked yet. Recipes are
// grouped into flavour families (spherical k-means over ingredient TF-IDF
// blended with text embeddings); picks then rotate through families by
// Thompson sampling on the user's ratings, plus a minority lane of dishes
// similar to ones they loved.
//
// Offline evaluation on the user's history (27 ratings) found no model that
// predicts which new dish they'll love better than chance, so the design is
// exploration-first rather than "more of the same".
package recommend

import (
	"math"
	"regexp"
	"strings"
)

// Recipe is the recommender's view of a library recipe.
type Recipe struct {
	ID          int
	Title       string
	Cuisine     string
	Tags        []string
	Ingredients []string
	Minutes     int
	// Embedding is optional; recipes without one fall back to TF-IDF only.
	Embedding []float32
}

var stopIngredients = map[string]bool{
	"salt": true, "black pepper": true, "pepper": true, "water": true, "oil": true,
	"cooking oil": true, "vegetable oil": true, "olive oil": true, "sunflower oil": true,
	"neutral oil": true, "sugar": true, "ice": true,
}

var titleStop = map[string]bool{
	"with": true, "and": true, "the": true, "for": true, "one": true, "two": true, "style": true,
	"inspired": true, "easy": true, "quick": true, "classic": true, "authentic": true, "spicy": true,
	"creamy": true, "homemade": true, "simple": true, "single": true, "serve": true, "solo": true,
	"tasty": true, "healthy": true, "traditional": true, "from": true, "in": true, "of": true,
	"on": true, "de": true, "la": true, "al": true, "con": true,
}

var (
	descriptors = regexp.MustCompile(`\b(fresh|freshly|chopped|minced|sliced|diced|ground|large|small|medium|boneless|skinless|finely|roughly|grated|crushed|whole|dried|extra virgin|to taste|optional|cooked|raw|organic|ripe|thinly|thick|cut)\b`)
	parens      = regexp.MustCompile(`\(.*?\)`)
	nonLetters  = regexp.MustCompile(`[^a-z' -]+`)
	titleSplit  = regexp.MustCompile(`[^\p{L}]+`)
)

func singular(w string) string {
	if len(w) <= 4 || !strings.HasSuffix(w, "s") || strings.HasSuffix(w, "ss") ||
		strings.HasSuffix(w, "us") || strings.HasSuffix(w, "is") {
		return w
	}
	switch {
	case w == "leaves" || w == "halves" || w == "loaves":
		return strings.TrimSuffix(w, "ves") + "f"
	case strings.HasSuffix(w, "ies"):
		return strings.TrimSuffix(w, "ies") + "y"
	case strings.HasSuffix(w, "oes"):
		return strings.TrimSuffix(w, "es")
	}
	return strings.TrimSuffix(w, "s")
}

func normIngredient(name string) string {
	n := strings.ToLower(name)
	n = parens.ReplaceAllString(n, " ")
	if i := strings.Index(n, ","); i >= 0 {
		n = n[:i]
	}
	n = descriptors.ReplaceAllString(n, " ")
	n = nonLetters.ReplaceAllString(n, " ")
	words := strings.Fields(n)
	for i, w := range words {
		words[i] = singular(w)
	}
	return strings.Join(words, " ")
}

// features returns weighted terms: ingredients, dish words from the title,
// cuisine (weighted up: it was the strongest single signal offline) and tags.
func features(r Recipe) map[string]float64 {
	f := map[string]float64{}
	add := func(k string, w float64) {
		if w > f[k] {
			f[k] = w
		}
	}
	for _, ing := range r.Ingredients {
		if n := normIngredient(ing); n != "" && !stopIngredients[n] {
			add("i:"+n, 1)
		}
	}
	cuisine := strings.ToLower(r.Cuisine)
	if cuisine != "" {
		add("c:"+cuisine, 1.6)
	}
	for _, w := range titleSplit.Split(strings.ToLower(parens.ReplaceAllString(r.Title, " ")), -1) {
		if t := singular(w); len(t) > 2 && !titleStop[t] && !strings.Contains(cuisine, t) {
			add("t:"+t, 0.8)
		}
	}
	for _, t := range r.Tags {
		add("g:"+strings.ToLower(t), 0.4)
	}
	return f
}

// tfidf builds L2-normalised TF-IDF vectors over terms that occur in at least
// two recipes (a term unique to one recipe can't make two recipes similar).
func tfidf(recipes []Recipe) (vecs [][]float64, vocab []string) {
	feats := make([]map[string]float64, len(recipes))
	df := map[string]int{}
	for i, r := range recipes {
		feats[i] = features(r)
		for k := range feats[i] {
			df[k]++
		}
	}
	index := map[string]int{}
	for k, n := range df {
		if n >= 2 {
			index[k] = len(vocab)
			vocab = append(vocab, k)
		}
	}
	n := float64(len(recipes))
	vecs = make([][]float64, len(recipes))
	for i, f := range feats {
		v := make([]float64, len(vocab))
		for k, w := range f {
			if j, ok := index[k]; ok {
				v[j] = w * math.Log(n/float64(df[k]))
			}
		}
		vecs[i] = normalize(v)
	}
	return vecs, vocab
}

func normalize(v []float64) []float64 {
	var s float64
	for _, x := range v {
		s += x * x
	}
	if s == 0 {
		return v
	}
	s = math.Sqrt(s)
	for i := range v {
		v[i] /= s
	}
	return v
}

func dot(a, b []float64) float64 {
	var t float64
	for i := range a {
		t += a[i] * b[i]
	}
	return t
}

// embeddingShare is the embedding half's weight in the blended vector, so
// cos(blend) = 0.3·cos(tfidf) + 0.7·cos(embedding). The blend made flavour
// families more coherent across cuisines than TF-IDF alone.
const embeddingShare = 0.7

// blend concatenates the TF-IDF and embedding vectors so that their cosine
// is the weighted sum of the two similarities. A recipe missing its
// embedding gets zeros there and leans on TF-IDF.
func blend(t []float64, e []float64, dims int) []float64 {
	if dims == 0 {
		return t
	}
	out := make([]float64, 0, len(t)+dims)
	ws, we := math.Sqrt(1-embeddingShare), math.Sqrt(embeddingShare)
	for _, x := range t {
		out = append(out, x*ws)
	}
	if e == nil {
		out = append(out, make([]float64, dims)...)
	} else {
		for _, x := range e {
			out = append(out, x*we)
		}
	}
	return normalize(out)
}

func toFloat64(v []float32) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = float64(x)
	}
	return normalize(out)
}
