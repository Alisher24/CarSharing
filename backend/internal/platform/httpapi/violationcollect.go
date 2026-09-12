package httpapi

import (
	"regexp"
	"strconv"

	"github.com/Alisher24/CarSharing/backend/internal/contracts/formats"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
)

// The code and message a violation carries when the validator names no keyword more specific than
// "the value does not match", plus the codes the recognized keywords map to. The keywords
// themselves are kin-openapi's spelling and the codes are the contract's, so the two are declared
// separately even where they read alike.
const (
	codeInvalidField       = "invalid"
	codeRequiredField      = "required"
	codeUnknownField       = "unknown_field"
	codeUnexpectedBody     = "unexpected_body"
	messageSchemaMismatch  = "Value does not match the schema"
	messageRequiredMissing = "Required value is missing"
	schemaFieldRequired    = "required"
	schemaFieldProperties  = "properties"
)

// unsupportedProperty matches the reason kin-openapi gives for a field absent from the schema. The
// field name appears only in that sentence, so it is recovered from there to address the violation.
var unsupportedProperty = regexp.MustCompile(`^property ("(?:[^"\\]|\\.)*") is unsupported$`)

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

	failed := violation{
		Location:  location,
		Parameter: parameter,
		Code:      codeInvalidField,
		Message:   messageSchemaMismatch,
	}
	switch err.SchemaField {
	case schemaFieldRequired:
		failed.Code, failed.Message = codeRequiredField, messageRequiredMissing
	case schemaFieldProperties:
		failed.Code, failed.Message = codeUnknownField, "Unknown field"
	}
	if location == locationBody {
		failed.Pointer = bodyPointer(err)
	}
	return []violation{failed}
}

func bodyPointer(err *openapi3.SchemaError) *string {
	parts := err.JSONPointer()
	if err.SchemaField == schemaFieldProperties {
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
// still learns which part of the request was rejected. A body violation always addresses the root
// of the payload, because the error names no field within it.
func unspecificViolation(location, parameter string) violation {
	failed := violation{
		Location:  location,
		Parameter: parameter,
		Code:      codeInvalidField,
		Message:   messageSchemaMismatch,
	}
	if location == locationBody {
		root := ""
		failed.Pointer = &root
	}
	return failed
}
