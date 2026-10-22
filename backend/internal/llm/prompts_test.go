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
	if got := NormalizeCuisine("Nyonya"); got != "Malaysian" {
		t.Errorf("NormalizeCuisine(Nyonya) = %q, want Malaysian", got)
	}
	if got := NormalizeCuisine("Klingon"); got != "Klingon" {
		t.Errorf("unknown cuisine should pass through, got %q", got)
	}
}
