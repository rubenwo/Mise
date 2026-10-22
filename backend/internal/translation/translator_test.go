package translation

import (
	"strings"
	"testing"
)

func TestBuildPrompt_FollowsTranslateGemmaFormat(t *testing.T) {
	got := BuildPrompt("  spring onion ", "en", "nl")
	want := "You are a professional English (en) to Dutch (nl) translator. " +
		"Your goal is to accurately convey the meaning and nuances of the original English text " +
		"while adhering to Dutch grammar, vocabulary, and cultural sensitivities.\n" +
		"Produce only the Dutch translation, without any additional explanations or commentary. " +
		"Please translate the following English text into Dutch:\n\n\nspring onion"
	if got != want {
		t.Errorf("prompt mismatch:\n got: %q\nwant: %q", got, want)
	}
	if !strings.HasSuffix(got, "\n\n\nspring onion") {
		t.Error("text must follow two blank lines")
	}
}

func TestCleanTranslation(t *testing.T) {
	tests := []struct{ in, want string }{
		{"lente-ui", "lente-ui"},
		{"  \"Lente-ui.\"  ", "Lente-ui"},
		{"bosui\n\nNote: also called lente-ui", "bosui"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := cleanTranslation(tt.in); got != tt.want {
			t.Errorf("cleanTranslation(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
