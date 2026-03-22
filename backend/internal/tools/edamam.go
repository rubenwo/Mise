package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type EdamamClient struct {
	appID  string
	appKey string
	client *http.Client
}

func NewEdamamClient(appID, appKey string, timeout time.Duration) *EdamamClient {
	return &EdamamClient{
		appID:  appID,
		appKey: appKey,
		client: &http.Client{Timeout: timeout},
	}
}

type EdamamResult struct {
	Label       string   `json:"label"`
	Source      string   `json:"source"`
	URL         string   `json:"url"`
	Ingredients []string `json:"ingredients"`
	Calories    float64  `json:"calories"`
	TotalTime   float64  `json:"total_time"`
}

func (e *EdamamClient) Search(ctx context.Context, query string) ([]EdamamResult, error) {
	// Edamam API v2 requires credentials as query parameters (their API design).
	// Use url.Values to construct the URL explicitly; do NOT wrap client.Do errors
	// directly — net/http embeds the full URL (including credentials) in error strings.
	params := url.Values{}
	params.Set("type", "public")
	params.Set("q", query)
	params.Set("app_id", e.appID)
	params.Set("app_key", e.appKey)
	endpoint := "https://api.edamam.com/api/recipes/v2?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	// Edamam Recipe Search API v2 requires Accept and Edamam-Account-User headers.
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Edamam-Account-User", e.appID)

	resp, err := e.client.Do(req)
	if err != nil {
		// Do not wrap err: net/http embeds the full URL (with credentials) in the error string.
		return nil, fmt.Errorf("calling edamam API: request failed")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("edamam returned status %d: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Hits []struct {
			Recipe struct {
				Label           string   `json:"label"`
				Source          string   `json:"source"`
				URL             string   `json:"url"`
				IngredientLines []string `json:"ingredientLines"`
				Calories        float64  `json:"calories"`
				TotalTime       float64  `json:"totalTime"`
			} `json:"recipe"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	var results []EdamamResult
	for i, hit := range apiResp.Hits {
		if i >= 5 {
			break
		}
		results = append(results, EdamamResult{
			Label:       hit.Recipe.Label,
			Source:      hit.Recipe.Source,
			URL:         hit.Recipe.URL,
			Ingredients: hit.Recipe.IngredientLines,
			Calories:    hit.Recipe.Calories,
			TotalTime:   hit.Recipe.TotalTime,
		})
	}

	return results, nil
}
