package httpapi

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"

	"github.com/Alisher24/CarSharing/backend/internal/contracts/formats"
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
)

// violation names one field or parameter the request got wrong, so a client can correct the
// request without guessing which part the contract rejected.
type violation struct {
	Location  string  `json:"location"`
	Pointer   *string `json:"pointer,omitempty"`
	Parameter string  `json:"parameter,omitempty"`
	Code      string  `json:"code"`
	Message   string  `json:"message"`
}

// constraintsKey addresses the positional constraints carried from requireJSONRequestBody to the
// handlers that report them.
type constraintsKey struct{}

// locationOrder is the order violations are reported in: the body first, then the parameters from
// the most to the least specific to the resource.
var locationOrder = map[string]int{"body": 0, "query": 1, "path": 2, "header": 3}

// unsupportedProperty matches the reason kin-openapi gives for a field absent from the schema. The
// field name appears only in that sentence, so it is recovered from there to address the violation.
var unsupportedProperty = regexp.MustCompile(`^property ("(?:[^"\\]|\\.)*") is unsupported$`)

func bodyViolation(pointer, code, message string) violation {
	return violation{Location: "body", Pointer: &pointer, Code: code, Message: message}
}

// violationDetails renders violations into the free-form details object of the error contract, in a
// stable order and without repeats so that an identical request always produces an identical body.
func violationDetails(violations []violation) *servedapi.ApiError_Details {
	sortViolations(violations)
	data, _ := json.Marshal(struct {
		Violations []violation `json:"violations"`
	}{dedupeViolations(violations)})
	details := servedapi.ApiError_Details{}
	_ = details.UnmarshalJSON(data)
	return &details
}

// sortViolations orders violations by location, then by the field or parameter they address, then
// by code, so that neither map iteration nor validator ordering can vary the response.
func sortViolations(violations []violation) {
	sort.Slice(violations, func(i, j int) bool {
		a, b := violations[i], violations[j]
		switch {
		case locationOrder[a.Location] != locationOrder[b.Location]:
			return locationOrder[a.Location] < locationOrder[b.Location]
		case violationTarget(a) != violationTarget(b):
			return violationTarget(a) < violationTarget(b)
		default:
			return a.Code < b.Code
		}
	})
}

// dedupeViolations drops repeats of the same code on the same target. It requires sorted input, so
// that repeats are adjacent, and reuses the backing array because the sorted slice is not read again.
func dedupeViolations(violations []violation) []violation {
	unique := violations[:0]
	for _, v := range violations {
		if len(unique) > 0 && sameViolation(unique[len(unique)-1], v) {
			continue
		}
		unique = append(unique, v)
	}
	return unique
}

func sameViolation(a, b violation) bool {
	return a.Location == b.Location && violationTarget(a) == violationTarget(b) && a.Code == b.Code
}

// violationTarget is what the violation addresses: a JSON pointer into the body, or a parameter name.
func violationTarget(v violation) string {
	if v.Pointer != nil {
		return *v.Pointer
	}
	return v.Parameter
}

// collectViolations flattens a validator error into the violations the error contract reports. The
// validator nests one error per failing schema keyword inside multi-errors, so the tree is walked
// while the location and parameter learned from an enclosing request error are carried down.
func collectViolations(err error, location, parameter string) []violation {
	switch e := err.(type) {
	case openapi3.MultiError:
		var result []violation
		for _, child := range e {
			result = append(result, collectViolations(child, location, parameter)...)
		}
		return result
	case *openapi3filter.RequestError:
		if e.Parameter != nil {
			location, parameter = e.Parameter.In, e.Parameter.Name
		}
		return collectViolations(e.Err, location, parameter)
	case *openapi3.SchemaError:
		return schemaViolations(e, location, parameter)
	default:
		return []violation{unspecificViolation(location, parameter)}
	}
}

// schemaViolations reports a failing schema keyword. A keyword that failed because a nested schema
// failed is reported through that nested cause, which names the actual field.
func schemaViolations(err *openapi3.SchemaError, location, parameter string) []violation {
	if err.Origin != nil {
		if nested := collectViolations(err.Origin, location, parameter); len(nested) > 0 {
			return nested
		}
	}
	v := violation{Location: location, Parameter: parameter, Code: "invalid", Message: "Value does not match the schema"}
	switch err.SchemaField {
	case "required":
		v.Code, v.Message = "required", "Required value is missing"
	case "properties":
		v.Code, v.Message = "unknown_field", "Unknown field"
	}
	if location == "body" {
		v.Pointer = bodyPointer(err)
	}
	return []violation{v}
}

func bodyPointer(err *openapi3.SchemaError) *string {
	parts := err.JSONPointer()
	if err.SchemaField == "properties" {
		if quoted := unsupportedProperty.FindStringSubmatch(err.Reason); len(quoted) > 1 {
			if property, unquoteErr := strconv.Unquote(quoted[1]); unquoteErr == nil {
				parts = append(parts, property)
			}
		}
	}
	pointer := ""
	for _, part := range parts {
		pointer += "/" + formats.PointerSegment(part)
	}
	return &pointer
}

// unspecificViolation reports a validator error that names no schema keyword, so that a client
// still learns which part of the request was rejected.
func unspecificViolation(location, parameter string) violation {
	v := violation{Location: location, Parameter: parameter, Code: "invalid", Message: "Value does not match the schema"}
	if location == "body" {
		root := ""
		v.Pointer = &root
	}
	return v
}
