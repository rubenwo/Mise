package llm

import (
	"bytes"
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

// prop is one object property. Properties are kept in declaration order
// because constrained decoding generates fields in schema order: a recipe's
// title should come first, and a calculation must precede its result.
type prop struct {
	name   string
	schema any
}

// props marshals as a JSON object that preserves declaration order.
type props []prop

func (p props) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, kv := range p {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, _ := json.Marshal(kv.name)
		val, err := json.Marshal(kv.schema)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// object builds an object schema in which every property is required.
func object(ps ...prop) obj {
	required := make([]string, len(ps))
	for i, p := range ps {
		required[i] = p.name
	}
	return obj{"type": "object", "properties": props(ps), "required": required}
}

// optionalObject builds an object schema with the given required subset.
func optionalObject(required []string, ps ...prop) obj {
	return obj{"type": "object", "properties": props(ps), "required": required}
}

func arrayOf(items any, minItems int) obj {
	s := obj{"type": "array", "items": items}
	if minItems > 0 {
		s["minItems"] = minItems
	}
	return s
}

func enum(values ...string) obj { return obj{"type": "string", "enum": values} }

var (
	str     = obj{"type": "string"}
	num     = obj{"type": "number"}
	integer = obj{"type": "integer"}
	boolean = obj{"type": "boolean"}
)

// RecipeSchema constrains a generated recipe. When cuisines is non-empty,
// cuisine_type is limited to that list so the model cannot invent labels.
func RecipeSchema(cuisines []string) *Schema {
	cuisine := str
	if len(cuisines) > 0 {
		cuisine = enum(cuisines...)
	}
	ingredient := optionalObject([]string{"name", "amount", "unit"},
		prop{"name", str},
		prop{"amount", num},
		prop{"unit", enum(RecipeUnits...)},
		prop{"notes", str},
	)
	return mustSchema("recipe", object(
		prop{"title", str},
		prop{"description", str},
		prop{"cuisine_type", cuisine},
		prop{"prep_time_minutes", integer},
		prop{"cook_time_minutes", integer},
		prop{"servings", integer},
		prop{"difficulty", enum("easy", "medium", "hard")},
		prop{"ingredients", arrayOf(ingredient, 3)},
		prop{"instructions", arrayOf(str, 3)},
		prop{"dietary_restrictions", arrayOf(enum(DietaryLabels...), 0)},
		prop{"tags", arrayOf(enum(RecipeTags...), 0)},
	))
}

// DedupSchema constrains merged shopping-list items. "calculation" precedes
// "amount" so the model writes out the unit conversion before committing to
// a total; without it, non-reasoning models often drop an entry from the sum.
var DedupSchema = mustSchema("dedup", object(
	prop{"items", arrayOf(object(
		prop{"name", str},
		prop{"calculation", str},
		prop{"amount", num},
		prop{"unit", str},
	), 0)},
))

// ScanSchema constrains products identified in a photo. Wrapped in an object
// because OpenAI-compatible JSON modes require a top-level object.
var ScanSchema = mustSchema("scan", object(
	prop{"items", arrayOf(object(
		prop{"name", str},
		prop{"amount", num},
		prop{"unit", str},
		prop{"confident", boolean},
	), 0)},
))

// RankSchema constrains the library recommendation ranking reply. The
// explanation comes first so the model weighs the candidates before it
// commits to IDs.
var RankSchema = mustSchema("rank", object(
	prop{"explanation", str},
	prop{"recipe_ids", arrayOf(integer, 0)},
))

// TranslationSchema constrains a single supermarket-term translation.
var TranslationSchema = mustSchema("translation", object(
	prop{"translation", str},
))
