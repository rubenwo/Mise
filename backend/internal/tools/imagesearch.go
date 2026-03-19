package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type ImageSearcher struct {
	client *http.Client
}

func NewImageSearcher(timeout time.Duration) *ImageSearcher {
	return &ImageSearcher{client: &http.Client{Timeout: timeout}}
}

// SearchRecipeImage returns the first usable image URL for the given recipe title
// using DuckDuckGo's two-step image search (vqd token + image JSON endpoint).
func (s *ImageSearcher) SearchRecipeImage(ctx context.Context, recipeTitle string) (string, error) {
	query := recipeTitle + " food recipe"

	vqd, err := s.fetchVQD(ctx, query)
	if err != nil {
		return "", fmt.Errorf("fetching vqd token: %w", err)
	}

	return s.fetchImage(ctx, query, vqd)
}

// fetchVQD retrieves the vqd token DuckDuckGo requires for image searches.
func (s *ImageSearcher) fetchVQD(ctx context.Context, query string) (string, error) {
	u := "https://duckduckgo.com/?q=" + url.QueryEscape(query) + "&iax=images&ia=images"

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return "", err
	}

	// vqd is embedded in the page in several possible forms.
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`vqd="([^"]+)"`),
		regexp.MustCompile(`vqd=([0-9a-zA-Z%-]+)[&"'\s]`),
		regexp.MustCompile(`"vqd"\s*:\s*"([^"]+)"`),
	}
	for _, re := range patterns {
		if m := re.FindSubmatch(body); len(m) > 1 {
			return string(m[1]), nil
		}
	}

	return "", fmt.Errorf("vqd token not found in DuckDuckGo response")
}

// fetchImage calls DDG's image JSON endpoint and returns the first suitable image URL.
func (s *ImageSearcher) fetchImage(ctx context.Context, query, vqd string) (string, error) {
	u := fmt.Sprintf(
		"https://duckduckgo.com/i.js?q=%s&o=json&vqd=%s&f=,,,,,&p=1",
		url.QueryEscape(query), url.QueryEscape(vqd),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://duckduckgo.com/")
	req.Header.Set("Accept", "application/json, */*")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("image search returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []struct {
			Image     string `json:"image"`
			Thumbnail string `json:"thumbnail"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding image results: %w", err)
	}

	for _, r := range result.Results {
		img := r.Image
		if img == "" {
			img = r.Thumbnail
		}
		if img == "" {
			continue
		}
		lower := strings.ToLower(img)
		if strings.HasSuffix(lower, ".svg") || strings.Contains(lower, "favicon") {
			continue
		}
		// Prefer reasonably-sized images.
		if r.Width > 0 && r.Width < 100 {
			continue
		}
		return img, nil
	}

	return "", fmt.Errorf("no image found for %q", query)
}
