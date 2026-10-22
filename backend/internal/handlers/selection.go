package handlers

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/rubenwo/mise/internal/database"
)

// selectSuggestedIDs picks count recipe IDs from meta using a date-seeded algorithm
// that ensures cuisine diversity and surfaces older (less-visible) recipes.
func selectSuggestedIDs(meta []database.RecipeMeta, count int, now time.Time) []int {
	if len(meta) == 0 {
		return nil
	}
	if count > len(meta) {
		count = len(meta)
	}

	// Seed with today's date so suggestions are stable within a day.
	seed := int64(now.Year())*10000 + int64(now.Month())*100 + int64(now.Day())
	rng := rand.New(rand.NewSource(seed))

	type entry struct {
		id      int
		ageDays float64
	}

	// Group recipes by cuisine.
	pools := map[string][]entry{}
	for _, m := range meta {
		c := m.CuisineType
		if c == "" {
			c = "Other"
		}
		age := now.Sub(m.CreatedAt).Hours() / 24
		pools[c] = append(pools[c], entry{m.ID, age})
	}

	// Sort cuisines for a deterministic base, then shuffle with the seeded RNG.
	cuisines := make([]string, 0, len(pools))
	for c := range pools {
		cuisines = append(cuisines, c)
	}
	sort.Strings(cuisines)
	rng.Shuffle(len(cuisines), func(i, j int) { cuisines[i], cuisines[j] = cuisines[j], cuisines[i] })

	picked := make([]int, 0, count)
	for len(picked) < count {
		progress := false
		for _, c := range cuisines {
			if len(picked) >= count {
				break
			}
			pool := pools[c]
			if len(pool) == 0 {
				continue
			}
			// Weight each recipe by age: older recipes surface more easily.
			// w = 1 + ln(1 + ageDays/7) → ranges from 1.0 (new) to ~5 (1 year old).
			totalW := 0.0
			for _, e := range pool {
				totalW += 1.0 + math.Log1p(e.ageDays/7.0)
			}
			target := rng.Float64() * totalW
			cum, idx := 0.0, len(pool)-1
			for i, e := range pool {
				cum += 1.0 + math.Log1p(e.ageDays/7.0)
				if cum >= target {
					idx = i
					break
				}
			}
			picked = append(picked, pool[idx].id)
			pools[c] = append(pool[:idx], pool[idx+1:]...)
			progress = true
		}
		if !progress {
			break // all pools exhausted
		}
	}
	return picked
}

// weightedSample draws n items from pool without replacement, using the
// Efraimidis–Spirakis algorithm: each item gets key = rand^(1/weight), top n keys win.
func weightedSample(pool []database.RecipeSummary, n int, weightFor func(database.RecipeSummary) float64) []database.RecipeSummary {
	if n <= 0 || len(pool) == 0 {
		return nil
	}
	if n >= len(pool) {
		out := make([]database.RecipeSummary, len(pool))
		copy(out, pool)
		rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		return out
	}

	type keyed struct {
		s   database.RecipeSummary
		key float64
	}
	scored := make([]keyed, len(pool))
	for i, s := range pool {
		w := weightFor(s)
		if w <= 0 {
			w = 1e-9
		}
		u := rand.Float64()
		if u <= 0 {
			u = 1e-12
		}
		scored[i] = keyed{s: s, key: math.Pow(u, 1.0/w)}
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].key > scored[j].key })

	out := make([]database.RecipeSummary, n)
	for i := range n {
		out[i] = scored[i].s
	}
	return out
}

// diversifyCuisines swaps duplicate-cuisine picks for a different-cuisine
// alternative from the pool, if one is available. Soft preference, not a hard rule:
// recipes with empty cuisine_type are ignored (treated as "unknown, no swap").
func diversifyCuisines(selected, pool []database.RecipeSummary, weightFor func(database.RecipeSummary) float64) []database.RecipeSummary {
	if len(selected) < 2 {
		return selected
	}
	chosenIDs := make(map[int]bool, len(selected))
	for _, s := range selected {
		chosenIDs[s.ID] = true
	}

	cuisineCount := map[string]int{}
	for _, s := range selected {
		if s.CuisineType != "" {
			cuisineCount[s.CuisineType]++
		}
	}

	for i, s := range selected {
		if s.CuisineType == "" || cuisineCount[s.CuisineType] < 2 {
			continue
		}
		var best *database.RecipeSummary
		var bestKey float64
		for j := range pool {
			cand := pool[j]
			if chosenIDs[cand.ID] || cand.CuisineType == "" {
				continue
			}
			if cuisineCount[cand.CuisineType] > 0 {
				continue
			}
			w := weightFor(cand)
			if w <= 0 {
				continue
			}
			u := rand.Float64()
			if u <= 0 {
				u = 1e-12
			}
			k := math.Pow(u, 1.0/w)
			if best == nil || k > bestKey {
				c := cand
				best = &c
				bestKey = k
			}
		}
		if best != nil {
			cuisineCount[s.CuisineType]--
			cuisineCount[best.CuisineType]++
			delete(chosenIDs, s.ID)
			chosenIDs[best.ID] = true
			selected[i] = *best
		}
	}
	return selected
}
