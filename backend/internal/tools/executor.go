package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

type Executor struct {
	webSearcher   *WebSearcher
	dbSearcher    *DBSearcher
	edamamClient  *EdamamClient
}

func NewExecutor(ws *WebSearcher, ds *DBSearcher, ec *EdamamClient) *Executor {
	return &Executor{
		webSearcher:  ws,
		dbSearcher:   ds,
		edamamClient: ec,
	}
}

func (e *Executor) Execute(ctx context.Context, name string, args json.RawMessage) (string, error) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	var result any
	var err error

	switch name {
	case "web_search":
		result, err = e.webSearcher.Search(ctx, params.Query)
	case "db_search":
		result, err = e.dbSearcher.Search(ctx, params.Query)
	case "edamam_search":
		if e.edamamClient == nil {
			return "Edamam API is not configured", nil
		}
		result, err = e.edamamClient.Search(ctx, params.Query)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}

	if err != nil {
		return "", err
	}

	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshaling result: %w", err)
	}

	return string(data), nil
}
