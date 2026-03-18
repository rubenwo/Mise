package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type WebSearcher struct {
	client *http.Client
}

func NewWebSearcher(timeout time.Duration) *WebSearcher {
	return &WebSearcher{
		client: &http.Client{Timeout: timeout},
	}
}

type WebSearchResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

func (w *WebSearcher) Search(ctx context.Context, query string) ([]WebSearchResult, error) {
	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query+" recipe")

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching search results: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	var results []WebSearchResult
	doc.Find(".result").Each(func(i int, s *goquery.Selection) {
		if i >= 5 {
			return
		}

		title := strings.TrimSpace(s.Find(".result__title").Text())
		snippet := strings.TrimSpace(s.Find(".result__snippet").Text())
		link, _ := s.Find(".result__url").Attr("href")

		if title != "" {
			results = append(results, WebSearchResult{
				Title:   title,
				Snippet: snippet,
				URL:     link,
			})
		}
	})

	if len(results) == 0 {
		return []WebSearchResult{{
			Title:   "No results found",
			Snippet: "Try a different search query",
		}}, nil
	}

	return results, nil
}
