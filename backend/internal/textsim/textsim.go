// Package textsim provides lightweight string-similarity measures used for
// duplicate detection without an LLM.
package textsim

import "strings"

// Normalize lowercases s, replaces non-alphanumeric runes with spaces,
// and collapses runs of spaces.
func Normalize(s string) string {
	var b strings.Builder
	for _, ch := range strings.ToLower(s) {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// JaccardWords returns the Jaccard similarity of the word sets of a and b,
// ignoring tokens shorter than 3 characters (articles, prepositions, etc.).
func JaccardWords(a, b string) float64 {
	sa := wordSet(a)
	sb := wordSet(b)
	if len(sa) == 0 && len(sb) == 0 {
		return 1.0
	}
	intersection := 0
	for w := range sa {
		if _, ok := sb[w]; ok {
			intersection++
		}
	}
	union := len(sa) + len(sb) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func wordSet(s string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, w := range strings.Fields(s) {
		if len(w) >= 3 {
			set[w] = struct{}{}
		}
	}
	return set
}

// JaccardBigrams returns the Jaccard similarity of character-bigram sets,
// which handles minor spelling variations and word-order differences.
func JaccardBigrams(a, b string) float64 {
	sa := bigramSet(a)
	sb := bigramSet(b)
	if len(sa) == 0 && len(sb) == 0 {
		return 1.0
	}
	intersection := 0
	for k := range sa {
		if _, ok := sb[k]; ok {
			intersection++
		}
	}
	union := len(sa) + len(sb) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func bigramSet(s string) map[string]struct{} {
	set := map[string]struct{}{}
	runes := []rune(s)
	for i := 0; i+1 < len(runes); i++ {
		set[string(runes[i:i+2])] = struct{}{}
	}
	return set
}


// Levenshtein computes the edit distance between two strings.
func Levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}
	curr := make([]int, lb+1)
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
