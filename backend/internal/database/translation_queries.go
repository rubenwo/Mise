package database

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// GetTranslation returns a cached translation, or ("", nil) if not cached.
func (q *Queries) GetTranslation(ctx context.Context, sourceText, targetLang string) (string, error) {
	var translated string
	err := q.pool.QueryRow(ctx,
		"SELECT translated_text FROM translation_cache WHERE source_text = $1 AND target_lang = $2",
		sourceText, targetLang,
	).Scan(&translated)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return translated, err
}

// SetTranslation stores or updates a translation in the cache.
func (q *Queries) SetTranslation(ctx context.Context, sourceText, targetLang, translatedText string) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO translation_cache (source_text, target_lang, translated_text)
		VALUES ($1, $2, $3)
		ON CONFLICT (source_text, target_lang) DO UPDATE SET translated_text = $3`,
		sourceText, targetLang, translatedText)
	return err
}

// GetUntranslatedIngredientNames returns distinct ingredient names from all recipes
// that do not yet have a cached translation for targetLang.
// At most limit results are returned per call so each background run is bounded.
func (q *Queries) GetUntranslatedIngredientNames(ctx context.Context, targetLang string, limit int) ([]string, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT DISTINCT ing->>'name'
		FROM recipes, jsonb_array_elements(ingredients) AS ing
		WHERE ing->>'name' != ''
		  AND NOT EXISTS (
		    SELECT 1 FROM translation_cache
		    WHERE source_text = ing->>'name' AND target_lang = $1
		  )
		ORDER BY ing->>'name'
		LIMIT $2`,
		targetLang, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
