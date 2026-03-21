package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type ImageSearcher struct {
	client    *http.Client
	imagesDir string
}

func NewImageSearcher(timeout time.Duration, imagesDir string) *ImageSearcher {
	return &ImageSearcher{client: &http.Client{Timeout: timeout}, imagesDir: imagesDir}
}

// SearchAndDownloadRecipeImage searches for images, tries each candidate URL in order,
// and returns a local path once one downloads successfully. Falls back to the first
// remote URL if every download attempt fails.
func (s *ImageSearcher) SearchAndDownloadRecipeImage(ctx context.Context, recipeTitle, filename string) (string, error) {
	candidates, err := s.searchRecipeImageCandidates(ctx, recipeTitle)
	if err != nil {
		return "", err
	}

	if s.imagesDir == "" {
		return candidates[0], nil
	}

	if err := os.MkdirAll(s.imagesDir, 0755); err != nil {
		log.Printf("Image download: could not create images dir: %v", err)
		return candidates[0], nil
	}

	for i, remoteURL := range candidates {
		localURL, err := s.tryDownload(ctx, remoteURL, filename)
		if err != nil {
			log.Printf("Image download: candidate %d/%d skipped (%s): %v", i+1, len(candidates), remoteURL, err)
			continue
		}
		return localURL, nil
	}

	log.Printf("Image download: all %d candidate(s) failed for %q, using remote URL", len(candidates), recipeTitle)
	return candidates[0], nil
}

// tryDownload fetches remoteURL and saves it to disk. Returns the local /images/ path on
// success, or an error if the server returns a non-2xx status or the transfer fails.
func (s *ImageSearcher) tryDownload(ctx context.Context, remoteURL, filename string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", remoteURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	ext := extFromContentType(resp.Header.Get("Content-Type"))
	if ext == "" {
		ext = extFromURL(remoteURL)
	}
	if ext == "" {
		ext = ".jpg"
	}

	localPath := filepath.Join(s.imagesDir, filename+ext)
	f, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, io.LimitReader(resp.Body, 10*1024*1024)); err != nil {
		_ = os.Remove(localPath)
		return "", fmt.Errorf("write file: %w", err)
	}

	return "/images/" + filename + ext, nil
}

func extFromContentType(ct string) string {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "jpeg") || strings.Contains(ct, "jpg"):
		return ".jpg"
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "gif"):
		return ".gif"
	default:
		return ""
	}
}

func extFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	lower := strings.ToLower(u.Path)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif"} {
		if strings.HasSuffix(lower, ext) {
			if ext == ".jpeg" {
				return ".jpg"
			}
			return ext
		}
	}
	return ""
}

// SearchRecipeImage returns the first usable image URL for the given recipe title.
func (s *ImageSearcher) SearchRecipeImage(ctx context.Context, recipeTitle string) (string, error) {
	candidates, err := s.searchRecipeImageCandidates(ctx, recipeTitle)
	if err != nil {
		return "", err
	}
	return candidates[0], nil
}

// searchRecipeImageCandidates returns all usable image URLs found for the recipe title.
func (s *ImageSearcher) searchRecipeImageCandidates(ctx context.Context, recipeTitle string) ([]string, error) {
	query := recipeTitle + " food recipe"

	vqd, err := s.fetchVQD(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("fetching vqd token: %w", err)
	}

	return s.fetchImageCandidates(ctx, query, vqd)
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

// fetchImageCandidates calls DDG's image JSON endpoint and returns all suitable image URLs.
func (s *ImageSearcher) fetchImageCandidates(ctx context.Context, query, vqd string) ([]string, error) {
	u := fmt.Sprintf(
		"https://duckduckgo.com/i.js?q=%s&o=json&vqd=%s&f=,,,,,&p=1",
		url.QueryEscape(query), url.QueryEscape(vqd),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://duckduckgo.com/")
	req.Header.Set("Accept", "application/json, */*")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("image search returned %d: %s", resp.StatusCode, string(body))
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
		return nil, fmt.Errorf("decoding image results: %w", err)
	}

	var candidates []string
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
		if r.Width > 0 && r.Width < 100 {
			continue
		}
		candidates = append(candidates, img)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no image found for %q", query)
	}
	return candidates, nil
}
