// Package translation provides LLM-backed text translation with DB caching.
// Translations are cached so repeated calls for the same text are instant.
//
// Provider selection: only providers tagged "translation" are used. The prompt
// depends on the model: TranslateGemma gets its required template, other
// models are asked for the supermarket search term (see translate).
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

	translated, err := translate(ctx, client, text, targetLang)
	if err != nil {
		log.Printf("translation: LLM call failed for %q: %v", text, err)
		return text
	}
	if translated == "" {
		return text
	}

	if err := t.queries.SetTranslation(ctx, text, targetLang, translated); err != nil {
		log.Printf("translation: failed to cache %q -> %q: %v", text, translated, err)
	}
	return translated
}

// TranslateMany translates each text, returning results in input order.
// Items are translated one at a time: dedicated translation models are trained
// on single passages, and every result is cached, so repeat runs are cheap.
func (t *Translator) TranslateMany(ctx context.Context, texts []string, targetLang string) []string {
	out := make([]string, len(texts))
	for i, text := range texts {
		if ctx.Err() != nil {
			out[i] = text
			continue
		}
		out[i] = t.Translate(ctx, text, targetLang)
	}
	return out
}

// translate picks the prompt that suits the provider's model.
//
// TranslateGemma must be prompted with its own template, but it translates
// bare ingredient names poorly ("spring onion" -> "jonagold"). A general
// instruction model asked for the supermarket search term does far better
// (11/12 exact matches vs 7-8/12 with gemma4:26b in the e2e suite), so that
// is the default for any other model.
func translate(ctx context.Context, client *llm.Client, text, targetLang string) (string, error) {
	if strings.Contains(strings.ToLower(client.Model()), "translategemma") {
		resp, err := client.Chat(ctx, []llm.Message{{Role: "user", Content: BuildPrompt(text, "en", targetLang)}}, nil)
		if err != nil {
			return "", err
		}
		return cleanTranslation(resp.Message.Content), nil
	}

	resp, err := client.ChatJSON(ctx, []llm.Message{
		{Role: "system", Content: BuildSupermarketPrompt(targetLang)},
		{Role: "user", Content: text},
	}, llm.TranslationSchema)
	if err != nil {
		return "", err
	}
	var result struct {
		Translation string `json:"translation"`
	}
	if err := json.Unmarshal([]byte(resp.Message.Content), &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	return cleanTranslation(result.Translation), nil
}

// BuildSupermarketPrompt asks for the product name a shopper would search for.
// The Dutch examples are tuned for Albert Heijn ordering.
func BuildSupermarketPrompt(targetLang string) string {
	examples := ""
	if targetLang == "nl" {
		examples = ` Examples: "minced garlic" -> "knoflook", "self-raising flour" -> "zelfrijzend bakmeel", "scallops" -> "coquilles", "double cream" -> "slagroom".`
	}
	return fmt.Sprintf(`Translate an English recipe ingredient into the %s product name a shopper would type into a supermarket search box. Use the everyday supermarket term, singular or plural as it is sold, without amounts, preparation or brand.%s Reply as {"translation": "<term>"}.`, langName(targetLang), examples)
}

// BuildPrompt renders TranslateGemma's prompt template
// (https://ollama.com/library/translategemma).
func BuildPrompt(text, sourceLang, targetLang string) string {
	src := fmt.Sprintf("%s (%s)", langName(sourceLang), sourceLang)
	dst := fmt.Sprintf("%s (%s)", langName(targetLang), targetLang)
	return fmt.Sprintf("You are a professional %s to %s translator. "+
		"Your goal is to accurately convey the meaning and nuances of the original %s text "+
		"while adhering to %s grammar, vocabulary, and cultural sensitivities.\n"+
		"Produce only the %s translation, without any additional explanations or commentary. "+
		"Please translate the following %s text into %s:\n\n\n%s",
		src, dst, langName(sourceLang), langName(targetLang), langName(targetLang),
		langName(sourceLang), langName(targetLang), strings.TrimSpace(text))
}

// cleanTranslation keeps the first line of the reply and strips surrounding
// quotes and a trailing period, which models sometimes add to short phrases.
func cleanTranslation(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.Trim(strings.TrimSpace(s), `"'`)
	s = strings.TrimSuffix(s, ".")
	return strings.TrimSpace(s)
}

// langName converts a BCP-47 language code to a human-readable name for prompts.
func langName(code string) string {
	switch code {
	case "en":
		return "English"
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
