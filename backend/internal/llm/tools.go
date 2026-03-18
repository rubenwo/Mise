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
	Type       string                 `json:"type"`
	Required   []string               `json:"required"`
	Properties map[string]ToolProperty `json:"properties"`
}

type ToolProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

var WebSearchTool = Tool{
	Type: "function",
	Function: ToolFunction{
		Name:        "web_search",
		Description: "Search the web for recipe ideas, cooking techniques, ingredient combinations, or cuisine information. Use this to find inspiration and trending recipes.",
		Parameters: ToolParameters{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]ToolProperty{
				"query": {
					Type:        "string",
					Description: "The search query for finding recipe information",
				},
			},
		},
	},
}

var DBSearchTool = Tool{
	Type: "function",
	Function: ToolFunction{
		Name:        "db_search",
		Description: "Search the user's saved recipe database for existing recipes. Use this to avoid generating duplicates and to build on the user's preferences.",
		Parameters: ToolParameters{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]ToolProperty{
				"query": {
					Type:        "string",
					Description: "Search query to find existing recipes in the database",
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
