package llm

import "testing"

func TestPickLeastRepresentedCuisine(t *testing.T) {
	tests := []struct {
		counts map[string]int
		want   string
	}{
		{nil, ""},
		{map[string]int{"Thai": 3, "Italian": 1, "Mexican": 2}, "Italian"},
		{map[string]int{"Thai": 1, "Greek": 1, "Italian": 4}, "Greek"},
	}
	for _, tt := range tests {
		if got := PickLeastRepresentedCuisine(tt.counts); got != tt.want {
			t.Errorf("PickLeastRepresentedCuisine(%v) = %q, want %q", tt.counts, got, tt.want)
		}
	}
}

func TestNormalizeCuisine(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Nyonya", "Malaysian"},
		{"Malaysian Peranakan (Nyonya)", "Malaysian"},
		{"Nyonya/Malay", "Malaysian"},
		{"Malaysian-Asian", "Malaysian"},
		{"Thai-inspired", "Thai"},
		{"Thai-Inspired", "Thai"},
		{"Thai-Asian Fusion", "Thai"},
		{"Vietnamese-Inspired Asian Fusion", "Vietnamese"},
		{"korean style", "Korean"},
		{"japanese", "Japanese"},
		{"Asian-inspired", "Asian Fusion"},
		{"Asian", "Asian Fusion"},
		{"Japanese-Peruvian Fusion", "Nikkei"},
		{"Szechuan-inspired", "Szechuan"},
		{"Sichuan", "Szechuan"},
		{"Greek Mediterranean", "Greek"},
		{"  Dutch-Indonesian ", "Dutch-Indonesian"},
		{"Italian-American", "Italian-American"},
		{"Laotian-inspired", "Laotian"},
		{"Klingon", "Klingon"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormalizeCuisine(tt.in); got != tt.want {
			t.Errorf("NormalizeCuisine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
