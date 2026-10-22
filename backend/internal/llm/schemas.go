package llm

import (
	"encoding/json"
)

// Schema is a named JSON schema used to grammar-constrain model output.
type Schema struct {
	Name string
	JSON json.RawMessage
}

func mustSchema(name string, v any) *Schema {
	data, err := json.Marshal(v)
	if err != nil {
		panic("llm: invalid schema " + name + ": " + err.Error())
	}
	return &Schema{Name: name, JSON: data}
}

type obj = map[string]any

func object(required []string, props obj) obj {
	return obj{"type": "object", "properties": props, "required": required}
}

func arrayOf(items obj, minItems int) obj {
	s := obj{"type": "array", "items": items}
	if minItems > 0 {
		s["minItems"] = minItems
	}
	return s
}

var (
	str     = obj{"type": "string"}
	num     = obj{"type": "number"}
	integer = obj{"type": "integer"}
	boolean = obj{"type": "boolean"}
)

// RecipeSchema constrains a generated recipe. When cuisines is non-empty,
// cuisine_type is limited to that list so the model cannot invent labels.
func RecipeSchema(cuisines []string) *Schema {
	cuisine := obj{"type": "string"}
	if len(cuisines) > 0 {
		cuisine = obj{"type": "string", "enum": cuisines}
	}
	ingredient := object([]string{"name", "amount", "unit"}, obj{
		"name":   str,
		"amount": num,
		"unit":   str,
		"notes":  str,
	})
	return mustSchema("recipe", object(
		[]string{"title", "description", "cuisine_type", "prep_time_minutes", "cook_time_minutes",
			"servings", "difficulty", "ingredients", "instructions", "dietary_restrictions", "tags"},
		obj{
			"title":                str,
			"description":          str,
			"cuisine_type":         cuisine,
			"prep_time_minutes":    integer,
			"cook_time_minutes":    integer,
			"servings":             integer,
			"difficulty":           obj{"type": "string", "enum": []string{"easy", "medium", "hard"}},
			"ingredients":          arrayOf(ingredient, 3),
			"instructions":         arrayOf(str, 3),
			"dietary_restrictions": arrayOf(str, 0),
			"tags":                 arrayOf(str, 0),
		},
	))
}

// DedupSchema constrains merged shopping-list items.
var DedupSchema = mustSchema("dedup", object([]string{"items"}, obj{
	"items": arrayOf(object([]string{"name", "amount", "unit"}, obj{
		"name":   str,
		"amount": num,
		"unit":   str,
	}), 0),
}))

// ScanSchema constrains products identified in a photo. Wrapped in an object
// because OpenAI-compatible JSON modes require a top-level object.
var ScanSchema = mustSchema("scan", object([]string{"items"}, obj{
	"items": arrayOf(object([]string{"name", "amount", "unit", "confident"}, obj{
		"name":      str,
		"amount":    num,
		"unit":      str,
		"confident": boolean,
	}), 0),
}))

// RankSchema constrains the library recommendation ranking reply.
var RankSchema = mustSchema("rank", object([]string{"recipe_ids", "explanation"}, obj{
	"recipe_ids":  arrayOf(integer, 0),
	"explanation": str,
}))
