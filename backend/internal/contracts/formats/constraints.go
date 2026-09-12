package formats

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// WGS84 latitude bounds, which a GeoJSON position must stay within.
const (
	minLatitude = -90.0
	maxLatitude = 90.0
)

// Messages the extension rules answer a violating position with.
const (
	messageLatitudeOutOfRange = "Latitude must be between -90 and 90"
	messageModeOutOfOrder     = "Invoice lines must be ordered driving then paused"
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
	visitSchema(schema, value, "", &violations)
	return violations
}

// visitSchema reports the extension rules the schema declares for one value, then descends into the
// sub-schemas that value is made of. The pointer is the JSON Pointer of the value within the
// payload, so that a violation addresses the exact field.
func visitSchema(schema *openapi3.Schema, value any, pointer string, violations *[]Violation) {
	if schema == nil {
		return
	}

	visitObject(schema, value, pointer, violations)
	visitArray(schema, value, pointer, violations)
	visitBranches(schema, value, pointer, violations)
}

// visitObject descends into the properties the value carries, which is where a violation of a
// nested field is found.
func visitObject(schema *openapi3.Schema, value any, pointer string, violations *[]Violation) {
	object, ok := value.(map[string]any)
	if !ok {
		return
	}

	for key, child := range schema.Properties {
		field, exists := object[key]
		if !exists {
			continue
		}
		visitSchema(child.Value, field, fieldPointer(pointer, PointerSegment(key)), violations)
	}
}

// visitArray reports the positional extension rules of an array value, then descends into its
// items so that a violation of a nested element is found too.
func visitArray(schema *openapi3.Schema, value any, pointer string, violations *[]Violation) {
	items, ok := value.([]any)
	if !ok {
		return
	}

	checkCoordinateOrder(schema, items, pointer, violations)
	checkModeOrder(schema, items, pointer, violations)

	if schema.Items == nil {
		return
	}
	for index, item := range items {
		visitSchema(schema.Items.Value, item, fieldPointer(pointer, fmt.Sprint(index)), violations)
	}
}

// checkCoordinateOrder enforces x-coordinate-order, which declares that a GeoJSON position is a
// longitude followed by a latitude. Only the latitude carries a bound: the contract accepts a
// longitude of any width and relies on the schema for the pair's shape.
func checkCoordinateOrder(schema *openapi3.Schema, items []any, pointer string, violations *[]Violation) {
	if schema.Extensions["x-coordinate-order"] != "longitude-latitude" || len(items) != 2 {
		return
	}

	latitude, ok := items[1].(float64)
	if !ok || latitude >= minLatitude && latitude <= maxLatitude {
		return
	}

	*violations = append(*violations, Violation{
		Pointer: fieldPointer(pointer, "1"),
		Message: messageLatitudeOutOfRange,
	})
}

// checkModeOrder enforces x-mode-order, which declares the sequence of modes an array of invoice
// lines must be in.
func checkModeOrder(schema *openapi3.Schema, items []any, pointer string, violations *[]Violation) {
	order, ok := schema.Extensions["x-mode-order"].([]any)
	if !ok {
		return
	}

	for index, mode := range order {
		if index >= len(items) {
			continue
		}
		line, ok := items[index].(map[string]any)
		if !ok || line["mode"] == mode {
			continue
		}
		*violations = append(*violations, Violation{
			Pointer: fmt.Sprintf("%s/%d/mode", pointer, index),
			Message: messageModeOutOfOrder,
		})
	}
}

// visitBranches descends into the one branch of the schema a value belongs to, and into every
// composed allOf branch. A value belongs to the branch its discriminator value selects, or, when
// the schema has no discriminator, to the first branch the value validates against.
func visitBranches(schema *openapi3.Schema, value any, pointer string, violations *[]Violation) {
	branchSets := []openapi3.SchemaRefs{schema.OneOf, schema.AnyOf}
	for _, branches := range branchSets {
		for _, branch := range branches {
			if !selectedBranch(schema, branch, value) {
				continue
			}
			visitSchema(branch.Value, value, pointer, violations)
			break
		}
	}

	for _, branch := range schema.AllOf {
		visitSchema(branch.Value, value, pointer, violations)
	}
}

// selectedBranch reports whether a value belongs to one candidate branch of a discriminated union.
func selectedBranch(schema *openapi3.Schema, branch *openapi3.SchemaRef, value any) bool {
	if schema.Discriminator == nil {
		return branch.Value.VisitJSON(value) == nil
	}

	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	tag := branch.Value.Properties[schema.Discriminator.PropertyName]
	return tag != nil &&
		len(tag.Value.Enum) == 1 &&
		tag.Value.Enum[0] == object[schema.Discriminator.PropertyName]
}

func fieldPointer(pointer, segment string) string {
	return pointer + "/" + segment
}
