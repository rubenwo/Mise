package embedding

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode/utf8"
)

// tokenizer is EmbeddingGemma's SentencePiece-style BPE, read from the
// Hugging Face tokenizer.json: added tokens are matched on the raw text,
// spaces become "▁", and the rest is merged pair by pair in rank order, with
// characters outside the vocabulary falling back to byte tokens.
type tokenizer struct {
	vocab  map[string]int32
	merges map[uint64]merge // (left id, right id) -> merge
	added  map[byte][]addedToken
	bos    int32
	eos    int32
}

type merge struct {
	rank int32
	id   int32
}

type addedToken struct {
	content string
	id      int32
}

func pairKey(a, b int32) uint64 { return uint64(uint32(a))<<32 | uint64(uint32(b)) }

func loadTokenizer(path string) (*tokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw struct {
		AddedTokens []struct {
			ID      int32  `json:"id"`
			Content string `json:"content"`
		} `json:"added_tokens"`
		Model struct {
			Type         string           `json:"type"`
			ByteFallback bool             `json:"byte_fallback"`
			Vocab        map[string]int32 `json:"vocab"`
			Merges       [][2]string      `json:"merges"`
		} `json:"model"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if raw.Model.Type != "BPE" || !raw.Model.ByteFallback {
		return nil, fmt.Errorf("%s: want a byte-fallback BPE tokenizer, got %q", path, raw.Model.Type)
	}
	t := &tokenizer{
		vocab:  raw.Model.Vocab,
		merges: make(map[uint64]merge, len(raw.Model.Merges)),
		added:  map[byte][]addedToken{},
	}
	for rank, m := range raw.Model.Merges {
		a, okA := t.vocab[m[0]]
		b, okB := t.vocab[m[1]]
		id, ok := t.vocab[m[0]+m[1]]
		if !okA || !okB || !ok {
			continue
		}
		if _, seen := t.merges[pairKey(a, b)]; !seen {
			t.merges[pairKey(a, b)] = merge{rank: int32(rank), id: id}
		}
	}
	for _, a := range raw.AddedTokens {
		if a.Content != "" {
			t.added[a.Content[0]] = append(t.added[a.Content[0]], addedToken{a.Content, a.ID})
		}
	}
	for _, list := range t.added {
		// Longest first, so matching is leftmost-longest.
		slices.SortFunc(list, func(x, y addedToken) int { return len(y.content) - len(x.content) })
	}
	var ok1, ok2 bool
	t.bos, ok1 = t.vocab["<bos>"]
	t.eos, ok2 = t.vocab["<eos>"]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("%s: missing <bos> or <eos>", path)
	}
	return t, nil
}

// encode returns the token ids for text wrapped in <bos> … <eos>, keeping at
// most maxTokens ids in total.
func (t *tokenizer) encode(text string, maxTokens int) []int32 {
	ids := []int32{t.bos}
	start := 0
	for i := 0; i < len(text); {
		if tok, ok := t.matchAdded(text[i:]); ok {
			ids = t.bpe(ids, text[start:i])
			ids = append(ids, tok.id)
			i += len(tok.content)
			start = i
			continue
		}
		i++
	}
	ids = t.bpe(ids, text[start:])
	if len(ids) > maxTokens-1 {
		ids = ids[:maxTokens-1]
	}
	return append(ids, t.eos)
}

func (t *tokenizer) matchAdded(s string) (addedToken, bool) {
	for _, tok := range t.added[s[0]] {
		if strings.HasPrefix(s, tok.content) {
			return tok, true
		}
	}
	return addedToken{}, false
}

// bpe appends the ids of one stretch of text without added tokens.
func (t *tokenizer) bpe(ids []int32, text string) []int32 {
	if text == "" {
		return ids
	}
	text = strings.ReplaceAll(text, " ", "▁")
	syms := make([]int32, 0, utf8.RuneCountInString(text))
	for _, r := range text {
		if id, ok := t.vocab[string(r)]; ok {
			syms = append(syms, id)
			continue
		}
		var buf [utf8.UTFMax]byte
		for _, b := range buf[:utf8.EncodeRune(buf[:], r)] {
			syms = append(syms, t.vocab[fmt.Sprintf("<0x%02X>", b)])
		}
	}
	// Merge the lowest-ranked adjacent pair (leftmost on ties) until none is
	// left. Texts are a few hundred characters, so a linear scan is enough.
	for len(syms) > 1 {
		best, at := merge{rank: -1}, -1
		for i := 0; i+1 < len(syms); i++ {
			if m, ok := t.merges[pairKey(syms[i], syms[i+1])]; ok && (at < 0 || m.rank < best.rank) {
				best, at = m, i
			}
		}
		if at < 0 {
			break
		}
		syms[at] = best.id
		syms = slices.Delete(syms, at+1, at+2)
	}
	return append(ids, syms...)
}
