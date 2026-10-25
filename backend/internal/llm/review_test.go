package llm

import "testing"

func TestMatchesCountableKey(t *testing.T) {
	tests := []struct {
		name, key string
		want      bool
	}{
		{"tomatoes", "tomato", true},
		{"eggs", "egg", true},
		{"red onions", "onion", true},
		{"red bell peppers", "bell pepper", true},
		{"eggplant", "egg", false},
		{"potato starch", "potato", false},
		{"garlic powder", "garlic", false},
		{"lime leaves", "lime", false},
		{"limes", "lime", true},
		{"applesauce", "apple", false},
	}
	for _, tt := range tests {
		if got := matchesCountableKey(tt.name, tt.key); got != tt.want {
			t.Errorf("matchesCountableKey(%q, %q) = %v, want %v", tt.name, tt.key, got, tt.want)
		}
	}
}
