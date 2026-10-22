package textsim

import "testing"

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"kitten", "sitting", 3},
		{"", "abc", 3},
		{"abc", "", 3},
		{"same", "same", 0},
		{"café", "cafe", 1},
	}
	for _, tt := range tests {
		if got := Levenshtein(tt.a, tt.b); got != tt.want {
			t.Errorf("Levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	if got := Normalize("  Spaghetti alla   Carbonara! (v2)"); got != "spaghetti alla carbonara v2" {
		t.Errorf("Normalize = %q", got)
	}
}

func TestJaccard(t *testing.T) {
	if got := JaccardWords("red lentil soup", "lentil soup red"); got != 1 {
		t.Errorf("JaccardWords on reordered words = %v, want 1", got)
	}
	if got := JaccardWords("", ""); got != 1 {
		t.Errorf("JaccardWords on empty strings = %v, want 1", got)
	}
	if got := JaccardBigrams("abc", "xyz"); got != 0 {
		t.Errorf("JaccardBigrams on disjoint strings = %v, want 0", got)
	}
}
