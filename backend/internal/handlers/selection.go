package handlers

import (
	"cmp"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/rubenwo/mise/internal/database"
)

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
	slices.SortFunc(scored, func(a, b keyed) int { return cmp.Compare(b.key, a.key) })

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
