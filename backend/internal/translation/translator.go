// Package translation provides LLM-backed text translation with DB caching.
// Translations are cached so repeated calls for the same text are instant.
//
// Provider selection: if a provider is tagged "translation" it is preferred;
// otherwise any healthy pool client is used. To use a lighter/faster model for
// translation, add a provider with the "translation" tag in Settings.
package translation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/llm"
)

// Translator translates text using Ollama, with results cached in PostgreSQL.
type Translator struct {
	pool    *llm.ClientPool
	queries *database.Queries
}

// New creates a Translator backed by the given client pool and DB queries.
func New(pool *llm.ClientPool, queries *database.Queries) *Translator {
	return &Translator{pool: pool, queries: queries}
}

// Translate translates text from English to targetLang (e.g. "nl").
// Returns the original text unchanged if translation is unavailable or fails.
func (t *Translator) Translate(ctx context.Context, text, targetLang string) string {
	if text == "" || targetLang == "" || targetLang == "en" {
		return text
	}

	// Check DB cache first — avoids an LLM call for repeated ingredients.
	if cached, err := t.queries.GetTranslation(ctx, text, targetLang); err == nil && cached != "" {
		return cached
	}

	client := t.pool.AcquireWithTag("translation")
	if client == nil {
		log.Printf("translation: no translation provider available, using original text %q", text)
		return text
	}

	messages := []llm.Message{
		{
			Role:    "system",
			Content: fmt.Sprintf(`You translate food ingredient names from English to %s. Reply with ONLY valid JSON in this exact format: {"translation":"<result>"}`, langName(targetLang)),
		},
		{Role: "user", Content: text},
	}

	resp, err := client.ChatJSON(ctx, messages)
	if err != nil {
		log.Printf("translation: LLM call failed for %q: %v", text, err)
		return text
	}

	var result struct {
		Translation string `json:"translation"`
	}
	if err := json.Unmarshal([]byte(resp.Message.Content), &result); err != nil {
		log.Printf("translation: failed to parse JSON response for %q: %v", text, err)
		return text
	}
	translated := cleanTranslation(result.Translation)
	if translated == "" {
		return text
	}

	// Cache for future calls.
	if err := t.queries.SetTranslation(ctx, text, targetLang, translated); err != nil {
		log.Printf("translation: failed to cache %q -> %q: %v", text, translated, err)
	}

	return translated
}

// TranslateMany translates a slice of texts in a single LLM call for uncached items,
// returning results in the same order as the input.
func (t *Translator) TranslateMany(ctx context.Context, texts []string, targetLang string) []string {
	out := make([]string, len(texts))

	// Resolve cache hits and collect indices that need LLM translation.
	var missingIdx []int
	var missingTexts []string
	for i, text := range texts {
		if text == "" || targetLang == "" || targetLang == "en" {
			out[i] = text
			continue
		}
		if cached, err := t.queries.GetTranslation(ctx, text, targetLang); err == nil && cached != "" {
			out[i] = cached
			continue
		}
		out[i] = text // default to original
		missingIdx = append(missingIdx, i)
		missingTexts = append(missingTexts, text)
	}

	if len(missingTexts) == 0 {
		return out
	}

	client := t.pool.AcquireWithTag("translation")
	if client == nil {
		log.Printf("translation: no translation provider available, returning original texts")
		return out
	}

	// Build a numbered list so the model returns translations in order.
	var userMsg strings.Builder
	for i, text := range missingTexts {
		fmt.Fprintf(&userMsg, "%d. %s\n", i+1, text)
	}

	messages := []llm.Message{
		{
			Role: "system",
			Content: fmt.Sprintf(
				`You translate food ingredient names from English to %s. `+
					`The user sends a numbered list. Reply with ONLY valid JSON in this exact format: {"translations":["<result1>","<result2>","..."]} — one entry per input, in the same order.`,
				langName(targetLang),
			),
		},
		{Role: "user", Content: strings.TrimSpace(userMsg.String())},
	}

	resp, err := client.ChatJSON(ctx, messages)
	if err != nil {
		log.Printf("translation: batch LLM call failed: %v", err)
		return out
	}

	var result struct {
		Translations []string `json:"translations"`
	}
	if err := json.Unmarshal([]byte(resp.Message.Content), &result); err != nil {
		log.Printf("translation: failed to parse batch JSON response: %v", err)
		return out
	}

	for i, translated := range result.Translations {
		if i >= len(missingIdx) {
			break
		}
		translated = cleanTranslation(translated)
		if translated == "" {
			continue
		}
		origIdx := missingIdx[i]
		origText := texts[origIdx]
		out[origIdx] = translated
		if err := t.queries.SetTranslation(ctx, origText, targetLang, translated); err != nil {
			log.Printf("translation: failed to cache %q -> %q: %v", origText, translated, err)
		}
	}

	return out
}

// cleanTranslation strips whitespace and surrounding quotes from a model response.
func cleanTranslation(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	return strings.TrimSpace(s)
}

// langName converts a BCP-47 language code to a human-readable name for prompts.
func langName(code string) string {
	switch code {
	case "nl":
		return "Dutch"
	case "fr":
		return "French"
	case "de":
		return "German"
	case "es":
		return "Spanish"
	case "it":
		return "Italian"
	default:
		return code
	}
}
