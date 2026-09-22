package contracts_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Alisher24/CarSharing/backend/internal/contracts/formats"
	internalapi "github.com/Alisher24/CarSharing/backend/internal/contracts/internalapi"
	mailstubapi "github.com/Alisher24/CarSharing/backend/internal/contracts/mailstubapi"
	publicapi "github.com/Alisher24/CarSharing/backend/internal/contracts/publicapi"
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/getkin/kin-openapi/openapi3"
)

const (
	// resourceID is a UUIDv7, which the contract requires for a stored resource so that
	// identifiers sort by creation time.
	resourceID = "01994342-6ba7-7000-8000-000000000001"

	// commandID is a UUIDv4, which the contract requires for a client-chosen idempotency key.
	commandID = "11111111-1111-4111-8111-111111111111"
)

// The manifest naming every contract, which sits at the repository root three directories above this
// package and is the same file the generators read.
const manifestPath = "../../../openapi/contracts.json"

// contract is one entry of the manifest: the name its source, its generator configuration and its
// bundle carry, and whether the production boundary registers its operations.
type contract struct {
	Name               string `json:"name"`
	ServedByProduction bool   `json:"servedByProduction"`
}

// generatedPackages names the projection each contract is checked through, so these checks read the
// document the processes serve rather than the sources a second time. The manifest names the
// contracts and this map says how to read one, and the test below holds the two sets together.
var generatedPackages = map[string]func() (*openapi3.T, error){
	"public":   publicapi.GetSwagger,
	"internal": internalapi.GetSwagger,
	"mailstub": mailstubapi.GetSwagger,
}

// servedPackage is the projection of the contract the production boundary serves: the operations
// `openapi/served.codegen.yaml` lists, and the ones this repository promises are routed.
var servedPackage = servedapi.GetSwagger

// The OpenAPI version every source contract declares, how a status key from a contract is read,
// and the first status that makes a response an error rather than a result.
const (
	openAPIVersion   = "3.0.3"
	firstErrorStatus = 400
	statusBase       = 10
	statusBits       = 16
)

// transportHeaders are the headers every response declares, because the transport boundary answers
// each of them whether or not the operation itself does.
var transportHeaders = []string{"X-Request-ID", "Cache-Control"}

// implementationStatus is the status an operation the production boundary serves declares.
const implementationStatus = "implemented"

// implementationStatuses is the closed set an operation may declare. An operation is routed only
// once it is marked implemented, and the router tests hold the two halves to each other: a planned
// operation must answer as an unknown resource, an implemented one must not.
var implementationStatuses = map[any]bool{implementationStatus: true, "planned": true}

func TestContractInventorySchemasAndExamples(t *testing.T) {
	declared := declaredContracts(t)
	for _, one := range declared {
		t.Run(one.Name, func(t *testing.T) { checkContract(t, loadContract(t, one.Name), one.ServedByProduction) })
	}
	for name := range generatedPackages {
		if !declares(declared, name) {
			t.Errorf("the package of the contract %s is read and the manifest does not declare it", name)
		}
	}
}

// loadContract names the generated package a contract is checked through. A contract the manifest
// declares without one has nothing to read, which is a defect in the manifest rather than a reason to
// skip the checks.
func loadContract(t *testing.T, name string) func() (*openapi3.T, error) {
	t.Helper()
	load, generated := generatedPackages[name]
	if !generated {
		t.Fatalf("the manifest declares the contract %s and no generated package reads it", name)
	}
	return load
}

// declaredContracts reads the manifest, which is the one declaration of the contract set the
// generators project and these checks hold to their declarations.
func declaredContracts(t *testing.T) []contract {
	t.Helper()
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Contracts []contract `json:"contracts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Contracts) == 0 {
		t.Fatalf("%s declares no contract", manifestPath)
	}
	return manifest.Contracts
}

func declares(declared []contract, name string) bool {
	for _, one := range declared {
		if one.Name == name {
			return true
		}
	}
	return false
}

// checkContract holds one contract's document to everything this repository promises about it, and,
// for the contract the production boundary serves, to the operations that boundary registers.
func checkContract(t *testing.T, load func() (*openapi3.T, error), servedByProduction bool) {
	t.Helper()
	spec, err := load()
	if err != nil {
		t.Fatal(err)
	}
	checkContractDocument(t, spec)
	checkContractOperations(t, spec)
	checkComponentSchemas(t, spec)
	checkBodyLimits(t, spec)
	if servedByProduction {
		served, err := servedPackage()
		if err != nil {
			t.Fatal(err)
		}
		checkServedOperations(t, spec, served)
	}
}

// checkContractDocument holds the document itself to the version and the validity the generators
// and this repository's checks both assume.
func checkContractDocument(t *testing.T, spec *openapi3.T) {
	t.Helper()
	if spec.OpenAPI != openAPIVersion {
		t.Fatalf("OpenAPI %s", spec.OpenAPI)
	}
	if err := spec.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// checkContractOperations walks every operation, holds it to a known implementation status and
// checks the examples, error codes and shared bodies of every response it declares.
func checkContractOperations(t *testing.T, spec *openapi3.T) {
	t.Helper()
	shared := sharedResponses(spec)
	seen := sharedErrorBodies(spec)
	for path, pathItem := range spec.Paths.Map() {
		for method, operation := range pathItem.Operations() {
			checkOperation(t, method, path, operation, shared, seen)
		}
	}
}

// sharedResponses is the set of bodies the contract declares once under components.responses, which
// its operations reference rather than repeat.
func sharedResponses(spec *openapi3.T) map[*openapi3.Response]bool {
	shared := make(map[*openapi3.Response]bool, len(spec.Components.Responses))
	for _, response := range spec.Components.Responses {
		shared[response.Value] = true
	}
	return shared
}

// checkBodyLimits holds every operation that accepts a body to the one limit its surface declares: an
// operation that states none is bounded by a constant of the process rather than by the contract, and
// two values on one surface would mean the contract does not state the limit at all.
func checkBodyLimits(t *testing.T, spec *openapi3.T) {
	t.Helper()
	limit, declared := 0.0, ""
	for path, pathItem := range spec.Paths.Map() {
		for method, operation := range pathItem.Operations() {
			if operation.RequestBody == nil {
				continue
			}
			value, stated := operation.Extensions["x-body-limit"].(float64)
			if !stated {
				t.Errorf("%s %s accepts a body and declares no x-body-limit", method, path)
				continue
			}
			switch {
			case declared == "":
				limit, declared = value, method+" "+path
			case value != limit:
				t.Errorf("%s %s declares x-body-limit %v; %s declares %v", method, path, value, declared, limit)
			}
		}
	}
}

// checkServedOperations holds the production boundary to the statuses the contract declares: every
// operation marked implemented must be one the boundary registers, and every operation it registers
// must be marked implemented. The two documents the processes serve are compared rather than the text
// of the configuration, because the generated document spells an operation id as the Go name it
// becomes while `openapi/served.codegen.yaml` lists it as the contract declares it.
func checkServedOperations(t *testing.T, source, served *openapi3.T) {
	t.Helper()
	implemented := operationsOf(source, implementationStatus)
	registered := operationsOf(served, "")
	for _, operation := range difference(implemented, registered) {
		t.Errorf("%s is implemented and the production boundary does not serve it", operation)
	}
	for _, operation := range difference(registered, implemented) {
		t.Errorf("the production boundary serves %s and the contract does not mark it implemented", operation)
	}
}

// operationsOf names every operation of a document as "METHOD /path", or only those declaring the
// wanted status when one is given.
func operationsOf(spec *openapi3.T, status string) []string {
	var operations []string
	for path, pathItem := range spec.Paths.Map() {
		for method, operation := range pathItem.Operations() {
			if status != "" && operation.Extensions["x-implementation-status"] != status {
				continue
			}
			operations = append(operations, method+" "+path)
		}
	}
	return operations
}

// difference returns the entries of want that allowed does not hold, sorted.
func difference(want, allowed []string) []string {
	held := make(map[string]bool, len(allowed))
	for _, entry := range allowed {
		held[entry] = true
	}
	var missing []string
	for _, entry := range want {
		if !held[entry] {
			missing = append(missing, entry)
		}
	}
	sort.Strings(missing)
	return missing
}

func checkOperation(
	t *testing.T, method, path string, operation *openapi3.Operation,
	shared map[*openapi3.Response]bool, seen map[string]string,
) {
	t.Helper()
	if !implementationStatuses[operation.Extensions["x-implementation-status"]] {
		t.Errorf("%s %s declares no known implementation status", method, path)
	}
	if operation.RequestBody != nil {
		checkContentExamples(t, operation.OperationID+" request", operation.RequestBody.Value.Content)
	}
	for status, response := range operation.Responses.Map() {
		checkResponse(t, operation.OperationID, status, response.Value)
		checkErrorBodyIsDeclaredOnce(t, method, path, status, response.Value, shared, seen)
	}
}

// checkErrorBodyIsDeclaredOnce holds an error body two operations answer with to one declaration: the
// second copy is the place the two come to disagree, so a shared body belongs under
// components.responses, where the operations reference it instead of restating it.
func checkErrorBodyIsDeclaredOnce(
	t *testing.T, method, path, status string, response *openapi3.Response,
	shared map[*openapi3.Response]bool, seen map[string]string,
) {
	t.Helper()
	statusCode, ok := parseStatus(status)
	if !ok || statusCode < firstErrorStatus || shared[response] {
		return
	}
	declared := method + " " + path + " " + status
	body := errorBody(response)
	if first, repeated := seen[body]; repeated {
		t.Errorf("%s answers with the body %s declares; declare it once under components.responses", declared, first)
		return
	}
	seen[body] = declared
}

// sharedErrorBodies names every body the contract declares under components.responses, so a copy of
// one written into an operation is reported as the second declaration it is.
func sharedErrorBodies(spec *openapi3.T) map[string]string {
	declared := make(map[string]string, len(spec.Components.Responses))
	for name, response := range spec.Components.Responses {
		declared[errorBody(response.Value)] = "components.responses " + name
	}
	return declared
}

// errorBody reads what an error response says: the codes it answers with and the examples it carries,
// which together are what two operations declaring one body would come to disagree about.
func errorBody(response *openapi3.Response) string {
	codes, _ := json.Marshal(response.Extensions["x-error-codes"])
	examples := map[string]any{}
	for _, media := range response.Content {
		for name, example := range media.Examples {
			examples[name] = example.Value.Value
		}
		if media.Example != nil {
			examples["example"] = media.Example
		}
	}
	values, _ := json.Marshal(examples)
	return string(codes) + "|" + string(values)
}

func checkResponse(t *testing.T, operationID, status string, response *openapi3.Response) {
	t.Helper()
	checkContentExamples(t, operationID+" response "+status, response.Content)
	for _, header := range transportHeaders {
		if response.Headers[header] == nil {
			t.Errorf("%s %s lacks transport header %s", operationID, status, header)
		}
	}
	statusCode, ok := parseStatus(status)
	if !ok {
		t.Errorf("%s declares unreadable status %q", operationID, status)
		return
	}
	if statusCode >= firstErrorStatus {
		checkErrorCodes(t, operationID, status, response)
	}
}

// parseStatus reads a status key from a contract, which is the status as a decimal string.
func parseStatus(status string) (int, bool) {
	code, err := strconv.ParseInt(status, statusBase, statusBits)
	if err != nil {
		return 0, false
	}
	return int(code), true
}

// checkComponentSchemas checks every named schema once. The schemas are visited transitively, so a
// shared schema reached from two places is still checked only once.
func checkComponentSchemas(t *testing.T, spec *openapi3.T) {
	t.Helper()
	seen := map[*openapi3.Schema]bool{}
	for name, schema := range spec.Components.Schemas {
		checkSchema(t, spec, name, schema, seen)
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

func checkSchema(
	t *testing.T, spec *openapi3.T, name string, ref *openapi3.SchemaRef, seen map[*openapi3.Schema]bool,
) {
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
		checkDiscriminator(t, spec, name, schema)
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

// checkDiscriminator holds every tag a discriminated schema declares to a branch that exists, that
// does not inherit a closed leaf, and that carries the tag it is mapped to.
func checkDiscriminator(t *testing.T, spec *openapi3.T, name string, schema *openapi3.Schema) {
	t.Helper()
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
	for _, boundary := range wireFormatBoundaries() {
		t.Run(boundary.name, func(t *testing.T) {
			schema := spec.Components.Schemas[boundary.name].Value
			for _, value := range boundary.good {
				if err := schema.VisitJSON(value); err != nil {
					t.Errorf("valid %v rejected: %v", value, err)
				}
			}
			for _, value := range boundary.bad {
				if err := schema.VisitJSON(value); err == nil {
					t.Errorf("invalid %v accepted", value)
				}
			}
		})
	}
}

// wireFormat is one named schema of the public contract together with values that must be accepted
// under it and values that must not.
type wireFormat struct {
	name      string
	good, bad []any
}

// wireFormatBoundaries is the boundary set of the public contract's wire formats. Each entry pairs
// a schema with the values its version and precision rules admit and the near misses they refuse.
func wireFormatBoundaries() []wireFormat {
	return []wireFormat{
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
	}
}

// A replay returns the saved original status and body, so Idempotency-Replayed belongs only on
// responses that can be saved: successes and verified domain failures of operations that carry an
// idempotency key. Auth, validation, in-flight and technical rollback results are never saved.
func TestReplayHeaderOnlyWhereResultsAreSaved(t *testing.T) {
	for _, one := range declaredContracts(t) {
		t.Run(one.Name, func(t *testing.T) {
			spec, err := loadContract(t, one.Name)()
			if err != nil {
				t.Fatal(err)
			}
			for path, pathItem := range spec.Paths.Map() {
				for method, operation := range pathItem.Operations() {
					checkReplayHeader(t, method, path, operation)
				}
			}
		})
	}
}

func checkReplayHeader(t *testing.T, method, path string, operation *openapi3.Operation) {
	t.Helper()
	keyed := hasIdempotencyKey(operation)
	for status, response := range operation.Responses.Map() {
		saved := savesResult(keyed, status, response.Value)
		if declared := response.Value.Headers["Idempotency-Replayed"] != nil; declared != saved {
			t.Errorf("%s %s %s declares replay header %t, saved result %t", method, path, status, declared, saved)
		}
	}
}

// hasIdempotencyKey reports whether an operation can be replayed at all: it carries one of the two
// key headers, or its body carries a client-chosen command identifier.
func hasIdempotencyKey(operation *openapi3.Operation) bool {
	for _, parameter := range operation.Parameters {
		name := parameter.Value.Name
		if parameter.Value.In == "header" && (name == "Idempotency-Key" || name == "Delivery-Key") {
			return true
		}
	}
	if operation.RequestBody == nil {
		return false
	}
	for _, media := range operation.RequestBody.Value.Content {
		if hasCommandID(media.Schema, map[*openapi3.Schema]bool{}) {
			return true
		}
	}
	return false
}

// savesResult reports whether a response is the saved answer to a keyed request. Only a success or
// a verified domain failure is saved; an in-flight or technical rollback result is not.
func savesResult(keyed bool, status string, response *openapi3.Response) bool {
	if !keyed {
		return false
	}
	if strings.HasPrefix(status, "2") {
		return true
	}
	return status == "409" && savesDomainFailure(response)
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
