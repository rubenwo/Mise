package embedding

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The model files are too large to commit: these tests run when
// MISE_EMBEDDING_DIR points at a directory holding them (and, for Embed,
// MISE_ONNXRUNTIME_LIB at the ONNX Runtime library).

func modelDir(t *testing.T) string {
	dir := os.Getenv("MISE_EMBEDDING_DIR")
	if dir == "" {
		t.Skip("MISE_EMBEDDING_DIR not set")
	}
	return dir
}

// testdata/tokens.json holds ids from the Hugging Face tokenizers library.
func TestTokenizerMatchesReference(t *testing.T) {
	tok, err := loadTokenizer(filepath.Join(modelDir(t), tokenizerFile))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Text string  `json:"text"`
		IDs  []int32 `json:"ids"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := tok.encode(c.Text, maxTokens); !slices.Equal(got, c.IDs) {
			t.Errorf("encode(%q)\n got %v\nwant %v", c.Text, got, c.IDs)
		}
	}
	if got := tok.encode(cases[0].Text, 5); len(got) != 5 || got[0] != tok.bos || got[4] != tok.eos {
		t.Errorf("truncated to 5 tokens: %v", got)
	}
}

func TestEmbedRanksSimilarDishesCloser(t *testing.T) {
	lib := os.Getenv("MISE_ONNXRUNTIME_LIB")
	if lib == "" {
		t.Skip("MISE_ONNXRUNTIME_LIB not set")
	}
	e := New(modelDir(t), lib)
	if !e.Available() {
		t.Fatal("model files or runtime library missing")
	}
	vecs, err := e.Embed(context.Background(), []string{
		"Chicken laksa. Cuisine: Malaysian. Ingredients: chicken thigh, rice vermicelli, laksa paste, coconut milk.",
		"Beef stew. Cuisine: Dutch. Ingredients: beef chuck, onion, bay leaf, potatoes.",
		"Curry laksa. Cuisine: Malaysian. Ingredients: prawns, egg noodles, laksa paste, coconut milk, tofu puffs.",
	})
	if err != nil {
		t.Fatal(err)
	}
	dot := func(a, b []float32) (s float32) {
		for i := range a {
			s += a[i] * b[i]
		}
		return s
	}
	if laksas, other := dot(vecs[0], vecs[2]), dot(vecs[0], vecs[1]); laksas <= other {
		t.Errorf("laksa–laksa similarity %.3f not above laksa–stew %.3f", laksas, other)
	}
	if n := dot(vecs[1], vecs[1]); n < 0.999 || n > 1.001 {
		t.Errorf("vector not unit length: |v|² = %.4f", n)
	}
}
