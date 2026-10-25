package recommend

import (
	"math/rand/v2"
	"sync"
)

// clustering is the result of spherical k-means: unit-length centroids and
// each vector's family.
type clustering struct {
	centroids [][]float64
	assign    []int
}

// restarts is how many k-means++ runs are tried per k; the best (highest
// total similarity to its centroids) wins.
const restarts = 8

func inertia(x [][]float64, c clustering) float64 {
	var score float64
	for i, v := range x {
		score += dot(v, c.centroids[c.assign[i]])
	}
	return score
}

func kmeansOnce(x [][]float64, k int, rng *rand.Rand) clustering {
	n, dims := len(x), len(x[0])
	// k-means++: spread the initial centroids out.
	centroids := [][]float64{clone(x[rng.IntN(n)])}
	dist := make([]float64, n)
	for len(centroids) < k {
		var total float64
		for i, v := range x {
			dist[i] = 1 - maxSim(v, centroids)
			total += dist[i]
		}
		target := rng.Float64() * total
		pick := n - 1
		for i, d := range dist {
			if target -= d; target <= 0 {
				pick = i
				break
			}
		}
		centroids = append(centroids, clone(x[pick]))
	}

	assign := make([]int, n)
	for i := range assign {
		assign[i] = -1
	}
	for range 50 {
		changed := false
		for i, v := range x {
			best, bestSim := 0, -2.0
			for j, c := range centroids {
				if s := dot(v, c); s > bestSim {
					best, bestSim = j, s
				}
			}
			if assign[i] != best {
				assign[i], changed = best, true
			}
		}
		if !changed {
			break
		}
		for j := range centroids {
			sum := make([]float64, dims)
			members := 0
			for i, v := range x {
				if assign[i] == j {
					members++
					for d, val := range v {
						sum[d] += val
					}
				}
			}
			if members > 0 {
				centroids[j] = normalize(sum)
			} else {
				centroids[j] = clone(x[rng.IntN(n)]) // re-seed an empty family
			}
		}
	}
	return clustering{centroids: centroids, assign: assign}
}

// silhouette is the mean silhouette coefficient under cosine distance: how
// much closer each recipe is to its own family than to the nearest other one.
// sim is the precomputed pairwise cosine similarity matrix.
func silhouette(sim [][]float64, assign []int, k int) float64 {
	n := len(sim)
	var total float64
	sum := make([]float64, k)
	cnt := make([]int, k)
	for i := range n {
		clear(sum)
		clear(cnt)
		for j := range n {
			if j != i {
				sum[assign[j]] += 1 - sim[i][j]
				cnt[assign[j]]++
			}
		}
		own := assign[i]
		if cnt[own] == 0 {
			continue // a singleton contributes 0
		}
		a := sum[own] / float64(cnt[own])
		b := -1.0
		for c := range k {
			if c != own && cnt[c] > 0 {
				if d := sum[c] / float64(cnt[c]); b < 0 || d < b {
					b = d
				}
			}
		}
		if b < 0 {
			continue
		}
		total += (b - a) / max(a, b)
	}
	return total / float64(n)
}

// chooseFamilies clusters with each candidate k and keeps the one with the
// best silhouette. All (k, restart) runs execute in parallel, each with its
// own seeded RNG so the result is deterministic.
func chooseFamilies(x [][]float64, seed uint64) clustering {
	var ks []int
	for _, k := range []int{10, 14, 18, 22, 26} {
		if k*6 <= len(x) { // keep families meaningfully sized
			ks = append(ks, k)
		}
	}
	if len(ks) == 0 { // tiny library: one family per ~6 recipes
		ks = []int{max(1, len(x)/6)}
	}

	runs := make([][]clustering, len(ks))
	var wg sync.WaitGroup
	for a, k := range ks {
		runs[a] = make([]clustering, restarts)
		for r := range restarts {
			wg.Go(func() {
				rng := rand.New(rand.NewPCG(seed, uint64(k*1000+r)))
				runs[a][r] = kmeansOnce(x, k, rng)
			})
		}
	}
	// Every candidate k is scored on the same pairwise similarities, so
	// compute them once (O(n²·d)) while the k-means runs proceed.
	sim := make([][]float64, len(x))
	for i := range x {
		sim[i] = make([]float64, len(x))
		for j := range i {
			sim[i][j] = dot(x[i], x[j])
			sim[j][i] = sim[i][j]
		}
		sim[i][i] = 1
	}
	wg.Wait()

	var best clustering
	bestSil := -2.0
	for a, k := range ks {
		top := runs[a][0]
		for _, c := range runs[a][1:] {
			if inertia(x, c) > inertia(x, top) {
				top = c
			}
		}
		if len(ks) == 1 {
			return top
		}
		if s := silhouette(sim, top.assign, k); s > bestSil {
			best, bestSil = top, s
		}
	}
	return best
}

func maxSim(v []float64, cs [][]float64) float64 {
	best := -2.0
	for _, c := range cs {
		if s := dot(v, c); s > best {
			best = s
		}
	}
	return best
}

func clone(v []float64) []float64 { return append([]float64(nil), v...) }
