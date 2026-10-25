package recommend

import (
	"math"
	"math/rand/v2"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Model holds the library's flavour families. Building one costs a few
// seconds of CPU for ~500 recipes, so callers cache it until the library or
// its embeddings change.
type Model struct {
	recipes  []Recipe
	index    map[int]int
	vec      [][]float64 // blended TF-IDF + embedding, unit length
	emb      [][]float64 // embedding alone (nil per recipe when missing)
	vocab    []string
	tfidf    [][]float64
	families clustering
}

// Build clusters recipes into flavour families. seed makes it deterministic.
func Build(recipes []Recipe, seed uint64) *Model {
	m := &Model{recipes: recipes, index: make(map[int]int, len(recipes))}
	if len(recipes) == 0 {
		return m
	}
	m.tfidf, m.vocab = tfidf(recipes)
	dims := 0
	for _, r := range recipes {
		if len(r.Embedding) > 0 {
			dims = len(r.Embedding)
			break
		}
	}
	m.emb = make([][]float64, len(recipes))
	m.vec = make([][]float64, len(recipes))
	for i, r := range recipes {
		m.index[r.ID] = i
		if dims > 0 && len(r.Embedding) == dims {
			m.emb[i] = toFloat64(r.Embedding)
		}
		m.vec[i] = blend(m.tfidf[i], m.emb[i], dims)
	}
	m.families = chooseFamilies(m.vec, seed)
	return m
}

// Families returns the number of flavour families.
func (m *Model) Families() int { return len(m.families.centroids) }

// FamilyLabel describes a family by its most characteristic ingredients and
// dish words, e.g. "lemongrass · shallot · palm sugar".
func (m *Model) FamilyLabel(f int) string {
	c := m.families.centroids[f]
	type term struct {
		w    float64
		name string
	}
	// The first len(vocab) dimensions are the TF-IDF part of the blend.
	var terms []term
	for i, name := range m.vocab {
		if strings.HasPrefix(name, "i:") || strings.HasPrefix(name, "t:") {
			terms = append(terms, term{c[i], name[2:]})
		}
	}
	sort.Slice(terms, func(a, b int) bool { return terms[a].w > terms[b].w })
	var parts []string
	for _, t := range terms[:min(3, len(terms))] {
		parts = append(parts, t.name)
	}
	return strings.Join(parts, " · ")
}

// Feedback is what the user has done with one recipe.
type Feedback struct {
	CookCount int
	AvgRating *float64
	Skipped   int // planned, then marked "not making it"
	// Impressions counts days the recipe was recommended without being
	// added to a plan; repeat no-shows fade it out.
	Impressions int
}

// Lane says why a recipe was picked.
type Lane string

const (
	LaneSimilar Lane = "similar" // close to a dish the user rated highly
	LaneExplore Lane = "explore" // from a flavour family picked by Thompson sampling
)

// Pick is one recommendation.
type Pick struct {
	RecipeID int
	Lane     Lane
	// Reason is a short human explanation for the pick.
	Reason string
}

// Options tunes a recommendation run.
type Options struct {
	Limit int
	// Seed makes the run reproducible; use the date for picks that stay put
	// for a day.
	Seed uint64
	// Exclude never recommends these recipes (e.g. already in the plan).
	Exclude map[int]bool
	// SimilarShare is the fraction of picks from the "similar" lane.
	SimilarShare float64
}

const (
	// lovedRating is the minimum average rating that seeds the similar lane.
	lovedRating = 8.0
	// sameDishEmbedding / sameDishTFIDF: above this similarity to something
	// already cooked, a candidate is the same dish again, not something new.
	// (Offline: "Bibimbap for Two" vs the cooked solo bibimbap scored 0.80;
	// Doro Wat vs Misir Wot, related but different, 0.73.)
	sameDishEmbedding = 0.77
	sameDishTFIDF     = 0.60
	// samePickEmbedding keeps two picks in one run from being near-twins.
	samePickEmbedding = 0.75
)

// appeal maps a rating onto 0..1 using the user's scale: 7+ is good food,
// 6 is fine only for a quick dish, 5 or lower means they didn't like it.
func appeal(rating float64, minutes int) float64 {
	if rating == 6 && minutes > 0 && minutes < 15 {
		rating = 7
	}
	return math.Max(0, math.Min(1, (rating-4)/6))
}

// Recommend picks recipes the user hasn't cooked. A share comes from the
// similar lane (sampled from the 25 closest to loved dishes so it rotates);
// the rest visits families in Thompson-sampled order, one dish each,
// preferring dishes typical of their family and far from anything cooked.
func (m *Model) Recommend(fb map[int]Feedback, opt Options) []Pick {
	if len(m.recipes) == 0 || opt.Limit <= 0 {
		return nil
	}
	rng := rand.New(rand.NewPCG(opt.Seed, 0x72656373))
	k := m.Families()

	var cooked, loved []int
	alpha, beta, exposure := make([]float64, k), make([]float64, k), make([]float64, k)
	for f := range k {
		alpha[f], beta[f] = 1, 1
	}
	for i, r := range m.recipes {
		f := fb[r.ID]
		fam := m.families.assign[i]
		switch {
		case f.CookCount > 0:
			cooked = append(cooked, i)
			p := 0.6 // cooked but unrated: mildly positive
			if f.AvgRating != nil {
				p = appeal(*f.AvgRating, r.Minutes)
				if *f.AvgRating >= lovedRating {
					loved = append(loved, i)
				}
			}
			n := float64(f.CookCount)
			alpha[fam] += p * n
			beta[fam] += (1 - p) * n
			exposure[fam] += n
		case f.Skipped > 0:
			beta[fam] += 0.3 * float64(f.Skipped)
		}
	}

	sameDish := func(i int) bool {
		for _, j := range cooked {
			if m.emb[i] != nil && m.emb[j] != nil {
				if dot(m.emb[i], m.emb[j]) > sameDishEmbedding {
					return true
				}
			} else if dot(m.tfidf[i], m.tfidf[j]) > sameDishTFIDF {
				return true
			}
		}
		return false
	}
	var candidates []int
	for i, r := range m.recipes {
		if fb[r.ID].CookCount == 0 && !opt.Exclude[r.ID] && !sameDish(i) {
			candidates = append(candidates, i)
		}
	}

	decay := func(i int) float64 { return float64(fb[m.recipes[i].ID].Impressions) * math.Log(0.6) }
	var picks []Pick
	var picked []int
	tooClose := func(i int, perFamily int) bool {
		sameFamily := 0
		for _, p := range picked {
			if p == i {
				return true
			}
			if m.families.assign[p] == m.families.assign[i] {
				sameFamily++
			}
			a, b := m.emb[p], m.emb[i]
			if a == nil || b == nil {
				a, b = m.vec[p], m.vec[i]
			}
			if dot(a, b) > samePickEmbedding {
				return true
			}
		}
		return sameFamily >= perFamily
	}

	// Similar lane.
	similar := int(math.Round(float64(opt.Limit) * opt.SimilarShare))
	if len(loved) > 0 && similar > 0 {
		type near struct {
			i, because int
			sim        float64
		}
		var pool []near
		for _, i := range candidates {
			best := near{i: i, because: -1}
			for _, j := range loved {
				if s := dot(m.vec[i], m.vec[j]); s > best.sim {
					best.sim, best.because = s, j
				}
			}
			if best.because >= 0 {
				pool = append(pool, best)
			}
		}
		sort.Slice(pool, func(a, b int) bool { return pool[a].sim > pool[b].sim })
		pool = pool[:min(25, len(pool))]
		for len(picks) < similar {
			var open []near
			for _, p := range pool {
				if !tooClose(p.i, 1) {
					open = append(open, p)
				}
			}
			if len(open) == 0 {
				break
			}
			c := softPick(rng, len(open), func(x int) float64 { return open[x].sim + 0.15*decay(open[x].i) }, 0.05)
			p := open[c]
			picked = append(picked, p.i)
			because := m.recipes[p.because]
			reason := "Similar to " + because.Title
			if r := fb[because.ID].AvgRating; r != nil {
				reason += ", which you rated " + formatRating(*r)
			}
			picks = append(picks, Pick{RecipeID: m.recipes[p.i].ID, Lane: LaneSimilar, Reason: reason})
		}
	}

	// Exploration lane: Thompson-sample each family's appeal; never-tried
	// families get a small bonus so they surface early.
	type draw struct {
		f int
		t float64
	}
	order := make([]draw, k)
	for f := range k {
		t := betaSample(rng, alpha[f]+0.5, beta[f]+0.5)
		if exposure[f] == 0 {
			t *= 1.15
		}
		order[f] = draw{f, t}
	}
	sort.Slice(order, func(a, b int) bool { return order[a].t > order[b].t })
	for perFamily := 1; perFamily <= 2 && len(picks) < opt.Limit; perFamily++ {
		for _, d := range order {
			if len(picks) >= opt.Limit {
				break
			}
			var pool []int
			for _, i := range candidates {
				if m.families.assign[i] == d.f && !tooClose(i, perFamily) {
					pool = append(pool, i)
				}
			}
			if len(pool) == 0 {
				continue
			}
			c := softPick(rng, len(pool), func(x int) float64 {
				i := pool[x]
				typical := dot(m.vec[i], m.families.centroids[d.f])
				novelty := 1 - maxSimIdx(m.vec, i, cooked)
				return 0.6*typical + 0.4*novelty + 0.2*decay(i)
			}, 0.08)
			i := pool[c]
			picked = append(picked, i)
			reason := "New territory: " + m.FamilyLabel(d.f)
			if exposure[d.f] > 0 {
				// Mean appeal of what was cooked in this family (prior removed).
				liked := (alpha[d.f] - 1) / (alpha[d.f] + beta[d.f] - 2)
				switch {
				case liked >= 0.65:
					reason = "From a family you've enjoyed: " + m.FamilyLabel(d.f)
				case liked < 0.4:
					reason = "Another try at " + m.FamilyLabel(d.f)
				default:
					reason = "More from " + m.FamilyLabel(d.f)
				}
			}
			picks = append(picks, Pick{RecipeID: m.recipes[i].ID, Lane: LaneExplore, Reason: reason})
		}
	}

	// Small or uniform libraries can run out of varied picks; fill the rest
	// with any remaining candidates rather than returning short (plan
	// filling needs exactly Limit dishes).
	if len(picks) < opt.Limit {
		rest := slices.DeleteFunc(slices.Clone(candidates), func(i int) bool { return slices.Contains(picked, i) })
		rng.Shuffle(len(rest), func(a, b int) { rest[a], rest[b] = rest[b], rest[a] })
		for _, i := range rest[:min(len(rest), opt.Limit-len(picks))] {
			picks = append(picks, Pick{RecipeID: m.recipes[i].ID, Lane: LaneExplore, Reason: "Something different: " + m.FamilyLabel(m.families.assign[i])})
		}
	}

	// Spread the similar picks through the list instead of front-loading them.
	return interleave(picks)
}

func interleave(picks []Pick) []Pick {
	var sim, exp []Pick
	for _, p := range picks {
		if p.Lane == LaneSimilar {
			sim = append(sim, p)
		} else {
			exp = append(exp, p)
		}
	}
	out := make([]Pick, 0, len(picks))
	for len(sim) > 0 || len(exp) > 0 {
		for range 2 {
			if len(exp) > 0 {
				out, exp = append(out, exp[0]), exp[1:]
			}
		}
		if len(sim) > 0 {
			out, sim = append(out, sim[0]), sim[1:]
		}
	}
	return out
}

func maxSimIdx(vec [][]float64, i int, others []int) float64 {
	best := 0.0
	for _, j := range others {
		best = math.Max(best, dot(vec[i], vec[j]))
	}
	return best
}

// softPick samples an index with probability ∝ exp(score/temp).
func softPick(rng *rand.Rand, n int, score func(int) float64, temp float64) int {
	w := make([]float64, n)
	hi := math.Inf(-1)
	for i := range n {
		w[i] = score(i) / temp
		hi = math.Max(hi, w[i])
	}
	var total float64
	for i := range w {
		w[i] = math.Exp(w[i] - hi)
		total += w[i]
	}
	t := rng.Float64() * total
	for i, x := range w {
		if t -= x; t <= 0 {
			return i
		}
	}
	return n - 1
}

// betaSample draws from Beta(a, b) via two gamma draws.
func betaSample(rng *rand.Rand, a, b float64) float64 {
	x, y := gammaSample(rng, a), gammaSample(rng, b)
	return x / (x + y)
}

// gammaSample uses Marsaglia–Tsang (with the boost for shape < 1).
func gammaSample(rng *rand.Rand, k float64) float64 {
	if k < 1 {
		return gammaSample(rng, k+1) * math.Pow(rng.Float64(), 1/k)
	}
	d := k - 1.0/3
	c := 1 / math.Sqrt(9*d)
	for {
		x := rng.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1-0.0331*x*x*x*x || math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}

// formatRating renders 10 as "10" and 9.5 as "9.5".
func formatRating(r float64) string { return strconv.FormatFloat(r, 'f', -1, 64) }
