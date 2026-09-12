package formats

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type Violation struct {
	Pointer string
	Message string
}

// PointerSegment escapes one JSON Pointer segment so body violations address the exact field.
func PointerSegment(part string) string {
	return strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
}

// Constraints checks positional rules that OpenAPI 3.0 cannot express with tuple schemas.
// The rules are declared as extensions in the source contract and apply to examples and HTTP payloads.
func Constraints(schema *openapi3.Schema, value any) []Violation {
	var violations []Violation
	var visit func(*openapi3.Schema, any, string)
	visit = func(schema *openapi3.Schema, value any, pointer string) {
		if schema == nil {
			return
		}
		if object, ok := value.(map[string]any); ok {
			for key, child := range schema.Properties {
				if field, exists := object[key]; exists {
					visit(child.Value, field, pointer+"/"+PointerSegment(key))
				}
			}
		}
		if items, ok := value.([]any); ok {
			if schema.Extensions["x-coordinate-order"] == "longitude-latitude" && len(items) == 2 {
				if latitude, ok := items[1].(float64); ok && (latitude < -90 || latitude > 90) {
					violations = append(violations, Violation{pointer + "/1", "Latitude must be between -90 and 90"})
				}
			}
			if order, ok := schema.Extensions["x-mode-order"].([]any); ok {
				for index, mode := range order {
					if index < len(items) {
						if line, ok := items[index].(map[string]any); ok && line["mode"] != mode {
							violations = append(violations, Violation{fmt.Sprintf("%s/%d/mode", pointer, index), "Invoice lines must be ordered driving then paused"})
						}
					}
				}
			}
			if schema.Items != nil {
				for index, item := range items {
					visit(schema.Items.Value, item, fmt.Sprintf("%s/%d", pointer, index))
				}
			}
		}
		for _, branches := range []openapi3.SchemaRefs{schema.OneOf, schema.AnyOf} {
			for _, branch := range branches {
				if schema.Discriminator != nil {
					object, ok := value.(map[string]any)
					if !ok {
						continue
					}
					tag := branch.Value.Properties[schema.Discriminator.PropertyName]
					if tag == nil || len(tag.Value.Enum) != 1 || tag.Value.Enum[0] != object[schema.Discriminator.PropertyName] {
						continue
					}
				} else if branch.Value.VisitJSON(value) != nil {
					continue
				}
				visit(branch.Value, value, pointer)
				break
			}
		}
		for _, branch := range schema.AllOf {
			visit(branch.Value, value, pointer)
		}
	}
	visit(schema, value, "")
	return violations
}
