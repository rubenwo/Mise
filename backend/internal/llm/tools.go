package llm

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  ToolParameters `json:"parameters"`
}

type ToolParameters struct {
	Type       string                  `json:"type"`
	Required   []string                `json:"required"`
	Properties map[string]ToolProperty `json:"properties"`
}

type ToolProperty struct {
	Type        string     `json:"type"`
	Description string     `json:"description"`
	Items       *ToolItems `json:"items,omitempty"`
}

type ToolItems struct {
	Type string `json:"type"`
}

var WebSearchTool = Tool{
	Type: "function",
	Function: ToolFunction{
		Name:        "web_search",
		Description: "Look up cooking facts you do not already know: traditional ratios for an unfamiliar dish, the source of an obscure recipe, a substitution you cannot infer. Do NOT call for general recipe ideas you can already produce.",
		Parameters: ToolParameters{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]ToolProperty{
				"query": {
					Type:        "string",
					Description: "Specific factual query, e.g. \"authentic ragu bolognese ratio\"",
				},
			},
		},
	},
}

var DBSearchTool = Tool{
	Type: "function",
	Function: ToolFunction{
		Name:        "db_search",
		Description: "Check the user's saved library for an existing recipe before generating something similar. Call once at most.",
		Parameters: ToolParameters{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]ToolProperty{
				"query": {
					Type:        "string",
					Description: "Short keyword query, e.g. \"chicken curry\"",
				},
			},
		},
	},
}

var EdamamSearchTool = Tool{
	Type: "function",
	Function: ToolFunction{
		Name:        "edamam_search",
		Description: "Search the Edamam recipe database for detailed recipe information including ingredients, nutrition, and source URLs. Use this for accurate ingredient lists and nutritional data.",
		Parameters: ToolParameters{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]ToolProperty{
				"query": {
					Type:        "string",
					Description: "Recipe search query for the Edamam API",
				},
			},
		},
	},
}
