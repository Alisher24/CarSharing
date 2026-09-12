package contracts_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Alisher24/CarSharing/backend/internal/contracts/formats"
	internalapi "github.com/Alisher24/CarSharing/backend/internal/contracts/internalapi"
	mailstubapi "github.com/Alisher24/CarSharing/backend/internal/contracts/mailstubapi"
	publicapi "github.com/Alisher24/CarSharing/backend/internal/contracts/publicapi"
	"github.com/getkin/kin-openapi/openapi3"
)

const (
	// resourceID is a UUIDv7, which the contract requires for a stored resource so that
	// identifiers sort by creation time.
	resourceID = "01994342-6ba7-7000-8000-000000000001"

	// commandID is a UUIDv4, which the contract requires for a client-chosen idempotency key.
	commandID = "11111111-1111-4111-8111-111111111111"
)

var contracts = map[string]struct {
	load       func() (*openapi3.T, error)
	operations []string
}{
	"public": {publicapi.GetSwagger, []string{
		"GET /api/v1/health/live",
		"GET /api/v1/health/ready",
		"POST /api/v1/auth/register",
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/logout",
		"GET /api/v1/me",
		"GET /api/v1/me/current",
		"GET /api/v1/vehicles",
		"GET /api/v1/vehicles/{id}",
		"GET /api/v1/zones",
		"GET /api/v1/tariffs",
		"POST /api/v1/reservations",
		"POST /api/v1/reservations/{id}/cancel",
		"POST /api/v1/reservations/{id}/start",
		"POST /api/v1/rides/{id}/pause",
		"POST /api/v1/rides/{id}/resume",
		"POST /api/v1/rides/{id}/finish",
		"GET /api/v1/me/rides",
		"GET /api/v1/me/invoices",
		"GET /api/v1/me/invoices/{id}",
		"POST /api/v1/me/invoices/{id}/pay",
		"GET /api/v1/me/notifications",
		"POST /api/v1/me/notifications/{id}/read",
		"GET /api/v1/events",
		"GET /api/v1/me/events",
	}},
	"internal": {internalapi.GetSwagger, []string{
		"POST /internal/v1/simulation/tick",
		"POST /internal/v1/demo/actions",
	}},
	"mailstub": {mailstubapi.GetSwagger, []string{
		"POST /internal/v1/messages",
		"POST /internal/v1/demo/actions",
		"GET /api/v1/messages",
		"GET /api/v1/messages/{id}",
	}},
}

func TestContractInventorySchemasAndExamples(t *testing.T) {
	for name, contract := range contracts {
		t.Run(name, func(t *testing.T) {
			spec, err := contract.load()
			if err != nil {
				t.Fatal(err)
			}
			if spec.OpenAPI != "3.0.3" {
				t.Fatalf("OpenAPI %s", spec.OpenAPI)
			}
			if err := spec.Validate(context.Background()); err != nil {
				t.Fatal(err)
			}
			var operations []string
			for path, item := range spec.Paths.Map() {
				for method, op := range item.Operations() {
					operations = append(operations, method+" "+path)
					if !strings.Contains(path, "/health/") && op.Extensions["x-implementation-status"] != "planned" {
						t.Errorf("%s is not planned", path)
					}
					if op.RequestBody != nil {
						checkContentExamples(t, op.OperationID+" request", op.RequestBody.Value.Content)
					}
					for status, res := range op.Responses.Map() {
						checkContentExamples(t, op.OperationID+" response "+status, res.Value.Content)
						if res.Value.Headers["X-Request-ID"] == nil || res.Value.Headers["Cache-Control"] == nil {
							t.Errorf("%s %s lacks transport headers", op.OperationID, status)
						}
						if status >= "400" {
							checkErrorCodes(t, op.OperationID, status, res.Value)
						}
					}
				}
			}
			sort.Strings(operations)
			want := append([]string(nil), contract.operations...)
			sort.Strings(want)
			if strings.Join(operations, "\n") != strings.Join(want, "\n") {
				t.Fatalf("operation inventory differs:\n%s", strings.Join(operations, "\n"))
			}
			seen := map[*openapi3.Schema]bool{}
			for name, schema := range spec.Components.Schemas {
				checkSchema(t, spec, name, schema, seen)
			}
		})
	}
}

func checkContentExamples(t *testing.T, name string, content openapi3.Content) {
	t.Helper()
	for _, media := range content {
		if media.Schema == nil {
			t.Errorf("%s has no schema", name)
			continue
		}
		if media.Example == nil && len(media.Examples) == 0 {
			t.Errorf("%s has no example", name)
		}
		if media.Example != nil {
			checkExample(t, name, media.Schema.Value, media.Example)
		}
		for key, example := range media.Examples {
			checkExample(t, name+" "+key, media.Schema.Value, example.Value.Value)
		}
	}
}

func checkExample(t *testing.T, name string, schema *openapi3.Schema, value any) {
	t.Helper()
	if err := schema.VisitJSON(value, openapi3.MultiErrors()); err != nil {
		t.Errorf("%s: %v", name, err)
	}
	if violations := formats.Constraints(schema, value); len(violations) > 0 {
		t.Errorf("%s: %v", name, violations)
	}
}

func checkSchema(t *testing.T, spec *openapi3.T, name string, ref *openapi3.SchemaRef, seen map[*openapi3.Schema]bool) {
	t.Helper()
	if ref == nil || ref.Value == nil {
		t.Errorf("unresolved schema %s", name)
		return
	}
	if ref.Ref != "" && !strings.HasPrefix(ref.Ref, "#/components/schemas/") {
		t.Errorf("non-local schema reference %s", ref.Ref)
	}
	schema := ref.Value
	if seen[schema] {
		return
	}
	seen[schema] = true
	if schema.Type.Is("object") && (schema.AdditionalProperties.Has == nil || *schema.AdditionalProperties.Has) {
		t.Errorf("open object: %s", name)
	}
	if schema.Example != nil {
		checkExample(t, name, schema, schema.Example)
	}
	if schema.Discriminator != nil {
		for tag, target := range schema.Discriminator.Mapping {
			resolved := spec.Components.Schemas[strings.TrimPrefix(target.Ref, "#/components/schemas/")]
			if resolved == nil {
				t.Errorf("%s discriminator %s is unresolved", name, tag)
				continue
			}
			if len(resolved.Value.AllOf) > 0 {
				t.Errorf("%s inherits a closed leaf with allOf", target.Ref)
			}
			property := resolved.Value.Properties[schema.Discriminator.PropertyName]
			if property == nil || len(property.Value.Enum) != 1 || property.Value.Enum[0] != tag {
				t.Errorf("%s discriminator tag disagrees with %s", name, target.Ref)
			}
		}
	}
	for property, child := range schema.Properties {
		checkSchema(t, spec, name+"."+property, child, seen)
	}
	if schema.Items != nil {
		checkSchema(t, spec, name+"[]", schema.Items, seen)
	}
	for _, set := range []openapi3.SchemaRefs{schema.OneOf, schema.AnyOf, schema.AllOf} {
		for _, child := range set {
			checkSchema(t, spec, name, child, seen)
		}
	}
}

// transportCodeStatus is the one status each transport error code may be declared under. A domain
// code is absent because its status depends on the operation that reports it, so a code missing
// here is simply not checked for placement.
var transportCodeStatus = map[string]string{
	"MALFORMED_JSON":                   "400",
	"INVALID_HEADER":                   "400",
	"INVALID_CURSOR":                   "400",
	"IDEMPOTENCY_KEY_REQUIRED":         "400",
	"IDEMPOTENCY_KEY_INVALID":          "400",
	"AUTHENTICATION_REQUIRED":          "401",
	"INVALID_CREDENTIALS":              "401",
	"INTERNAL_AUTHENTICATION_REQUIRED": "401",
	"ORIGIN_NOT_ALLOWED":               "403",
	"CSRF_INVALID":                     "403",
	"RESOURCE_NOT_FOUND":               "404",
	"BODY_TOO_LARGE":                   "413",
	"UNSUPPORTED_MEDIA_TYPE":           "415",
	"VALIDATION_FAILED":                "422",
	"RATE_LIMITED":                     "429",
	"INTERNAL_ERROR":                   "500",
	"SERVICE_UNAVAILABLE":              "503",
}

func checkErrorCodes(t *testing.T, operation, status string, response *openapi3.Response) {
	t.Helper()
	codes, ok := response.Extensions["x-error-codes"].([]any)
	if !ok || len(codes) == 0 {
		t.Errorf("%s %s has no applicable error codes", operation, status)
		return
	}
	for _, code := range codes {
		expected, transport := transportCodeStatus[fmt.Sprint(code)]
		if transport && expected != status {
			t.Errorf("%s %s has misplaced %s", operation, status, code)
		}
	}
	for _, example := range response.Content["application/json"].Examples {
		data, _ := json.Marshal(example.Value.Value)
		var body struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(data, &body)
		found := false
		for _, code := range codes {
			if code == body.Code {
				found = true
			}
		}
		if !found {
			t.Errorf("%s error example %s is not applicable", operation, body.Code)
		}
	}
}

func TestWireFormatBoundaries(t *testing.T) {
	spec, err := publicapi.GetSwagger()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		good, bad []any
	}{
		{
			name: "ResourceId",
			good: []any{resourceID},
			bad: []any{
				commandID,
				"01994342-6BA7-7000-8000-000000000001",
				"01994342-6ba7-7000-7000-000000000001",
			},
		},
		{
			name: "CommandId",
			good: []any{commandID},
			bad:  []any{resourceID, `"` + commandID + `"`},
		},
		{
			name: "ExactInteger",
			good: []any{"0", "42", "9223372036854775807"},
			bad: []any{
				"-1", "01", "1.0",
				"9223372036854775808", "10000000000000000000", float64(42),
			},
		},
		{
			name: "EnergyDecimal",
			good: []any{"0", "0.000001", "1200.125"},
			bad:  []any{"-0", "01", "1.0", "1e3", "0.0000001"},
		},
		{
			name: "Timestamp",
			good: []any{"2026-09-12T07:15:30.123456Z"},
			bad: []any{
				"2026-09-12T07:15:30Z",
				"2026-09-12T07:15:30.1234567Z",
				"2026-09-12T07:15:30.123456+00:00",
				"2026-02-30T07:15:30.123456Z",
			},
		},
		{
			name: "Cursor",
			good: []any{"eyJ2IjoxfQ"},
			bad:  []any{"", "a=b", "a+b", "a/b"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := spec.Components.Schemas[tc.name].Value
			for _, value := range tc.good {
				if err := schema.VisitJSON(value); err != nil {
					t.Errorf("valid %v rejected: %v", value, err)
				}
			}
			for _, value := range tc.bad {
				if err := schema.VisitJSON(value); err == nil {
					t.Errorf("invalid %v accepted", value)
				}
			}
		})
	}
}

// A replay returns the saved original status and body, so Idempotency-Replayed belongs only on
// responses that can be saved: successes and verified domain failures of operations that carry an
// idempotency key. Auth, validation, in-flight and technical rollback results are never saved.
func TestReplayHeaderOnlyWhereResultsAreSaved(t *testing.T) {
	for name, contract := range contracts {
		t.Run(name, func(t *testing.T) {
			spec, err := contract.load()
			if err != nil {
				t.Fatal(err)
			}
			for path, item := range spec.Paths.Map() {
				for method, op := range item.Operations() {
					keyed := false
					for _, parameter := range op.Parameters {
						name := parameter.Value.Name
						if parameter.Value.In == "header" && (name == "Idempotency-Key" || name == "Delivery-Key") {
							keyed = true
						}
					}
					if op.RequestBody != nil {
						for _, media := range op.RequestBody.Value.Content {
							keyed = keyed || hasCommandID(media.Schema, map[*openapi3.Schema]bool{})
						}
					}
					for status, res := range op.Responses.Map() {
						saved := keyed && (strings.HasPrefix(status, "2") || status == "409" && savesDomainFailure(res.Value))
						if declared := res.Value.Headers["Idempotency-Replayed"] != nil; declared != saved {
							t.Errorf("%s %s %s declares replay header %t, saved result %t", method, path, status, declared, saved)
						}
					}
				}
			}
		})
	}
}

func hasCommandID(ref *openapi3.SchemaRef, seen map[*openapi3.Schema]bool) bool {
	if ref == nil || ref.Value == nil || seen[ref.Value] {
		return false
	}
	seen[ref.Value] = true
	for _, property := range []string{"tick_id", "action_id"} {
		if ref.Value.Properties[property] != nil {
			return true
		}
	}
	for _, set := range []openapi3.SchemaRefs{ref.Value.OneOf, ref.Value.AnyOf, ref.Value.AllOf} {
		for _, child := range set {
			if hasCommandID(child, seen) {
				return true
			}
		}
	}
	return false
}

func savesDomainFailure(response *openapi3.Response) bool {
	inFlight := map[string]bool{"IDEMPOTENCY_CONFLICT": true, "IDEMPOTENCY_IN_PROGRESS": true, "DELIVERY_CONFLICT": true}
	codes, _ := response.Extensions["x-error-codes"].([]any)
	for _, code := range codes {
		if !inFlight[fmt.Sprint(code)] {
			return true
		}
	}
	return false
}
