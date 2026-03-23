package handlers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/rubenwo/mise/internal/database"
	"github.com/rubenwo/mise/internal/translation"
)

// translationBatchSize caps how many ingredients are translated per run to keep
// each job short and LLM cost predictable.
const translationBatchSize = 50

// BackgroundTranslator pre-populates the translation cache on a cron-like
// schedule so ingredients are already translated when users open AH ordering.
type BackgroundTranslator struct {
	queries    *database.Queries
	translator *translation.Translator
	stop       chan struct{}
}

func NewBackgroundTranslator(q *database.Queries, t *translation.Translator) *BackgroundTranslator {
	return &BackgroundTranslator{queries: q, translator: t, stop: make(chan struct{})}
}

// Start launches the background translation loop. Schedule and language are
// read from app_settings on every tick so changes take effect without a restart.
func (b *BackgroundTranslator) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s := b.loadSettings(ctx)
				if !s.enabled || len(s.days) == 0 || s.timeHour < 0 {
					continue
				}

				now := time.Now()
				if !s.days[now.Weekday()] {
					continue
				}

				todayTarget := time.Date(now.Year(), now.Month(), now.Day(), s.timeHour, s.timeMin, 0, 0, now.Location())
				if now.Before(todayTarget) {
					continue
				}

				lastRun := b.loadLastRun(ctx)
				if !lastRun.IsZero() && lastRun.After(todayTarget) {
					continue
				}

				if err := b.queries.SetSetting(ctx, "background_translation_last_run", now.Format(time.RFC3339)); err != nil {
					log.Printf("BackgroundTranslator: failed to persist last run time: %v", err)
				}

				// Translate to the configured UI language; skip if English (nothing to translate).
				targetLang, _ := b.queries.GetSetting(ctx, "ui_language")
				if targetLang == "" || targetLang == "en" {
					log.Printf("BackgroundTranslator: ui_language is %q, nothing to translate", targetLang)
					continue
				}

				log.Printf("BackgroundTranslator: starting (lang=%s)", targetLang)
				b.runTranslation(ctx, targetLang)

			case <-b.stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (b *BackgroundTranslator) Stop() {
	close(b.stop)
}

type bgTranslationSettings struct {
	enabled  bool
	days     map[time.Weekday]bool
	timeHour int // -1 if not configured
	timeMin  int
}

func (b *BackgroundTranslator) loadSettings(ctx context.Context) bgTranslationSettings {
	s := bgTranslationSettings{timeHour: -1, days: map[time.Weekday]bool{}}

	if val, _ := b.queries.GetSetting(ctx, "background_translation_enabled"); val != "true" {
		return s
	}
	s.enabled = true

	if val, _ := b.queries.GetSetting(ctx, "background_translation_days"); val != "" {
		for _, part := range strings.Split(val, ",") {
			part = strings.TrimSpace(part)
			if n, err := strconv.Atoi(part); err == nil && n >= 0 && n <= 6 {
				s.days[time.Weekday(n)] = true
			}
		}
	}

	if val, _ := b.queries.GetSetting(ctx, "background_translation_time"); val != "" {
		var h, m int
		if _, err := fmt.Sscanf(val, "%d:%d", &h, &m); err == nil && h >= 0 && h <= 23 && m >= 0 && m <= 59 {
			s.timeHour = h
			s.timeMin = m
		}
	}

	return s
}

func (b *BackgroundTranslator) loadLastRun(ctx context.Context) time.Time {
	val, _ := b.queries.GetSetting(ctx, "background_translation_last_run")
	if val == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (b *BackgroundTranslator) runTranslation(ctx context.Context, targetLang string) {
	names, err := b.queries.GetUntranslatedIngredientNames(ctx, targetLang, translationBatchSize)
	if err != nil {
		log.Printf("BackgroundTranslator: failed to query untranslated ingredients: %v", err)
		return
	}
	if len(names) == 0 {
		log.Printf("BackgroundTranslator: all ingredients already translated to %s", targetLang)
		return
	}
	log.Printf("BackgroundTranslator: translating %d ingredient(s) to %s", len(names), targetLang)
	b.translator.TranslateMany(ctx, names, targetLang)
	log.Printf("BackgroundTranslator: finished translating to %s", targetLang)
}
