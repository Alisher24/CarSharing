package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Alisher24/CarSharing/backend/internal/contracts/formats"
	healthapi "github.com/Alisher24/CarSharing/backend/internal/contracts/healthapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
)

type violation struct {
	Location  string  `json:"location"`
	Pointer   *string `json:"pointer,omitempty"`
	Parameter string  `json:"parameter,omitempty"`
	Code      string  `json:"code"`
	Message   string  `json:"message"`
}

type constraintsKey struct{}

func bodyViolation(pointer, code, message string) violation {
	return violation{Location: "body", Pointer: &pointer, Code: code, Message: message}
}

// boundary applies the same transport contract to production and isolated contract routers.
// Authentication is supplied by the owning application; the health projection has no security requirements.
func boundary(spec *openapi3.T, next http.Handler, authenticate ...openapi3filter.AuthenticationFunc) http.Handler {
	routes, err := gorillamux.NewRouter(spec)
	if err != nil {
		panic(err)
	}
	var auth openapi3filter.AuthenticationFunc
	if len(authenticate) > 0 {
		auth = authenticate[0]
	}
	checked := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if violations, ok := r.Context().Value(constraintsKey{}).([]violation); ok && len(violations) > 0 {
			writeError(w, r, 422, "VALIDATION_FAILED", "Request validation failed", violations...)
			return
		}
		next.ServeHTTP(w, r)
	})
	validated := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		Options: openapi3filter.Options{MultiError: true, AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, _ nethttpmiddleware.ErrorHandlerOpts) {
			validationError(spec, w, r, err)
		},
	})(checked)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), middleware.RequestIDKey, uuid.NewString()))
		w.Header().Set("X-Request-ID", middleware.GetReqID(r.Context()))
		w.Header().Set("Cache-Control", "no-store")
		defer func() {
			if recovered := recover(); recovered != nil {
				writeError(w, r, 500, "INTERNAL_ERROR", "Internal server error")
			}
		}()
		route, pathParams, err := routes.FindRoute(r)
		if err != nil {
			validationError(spec, w, r, err)
			return
		}
		security := route.Operation.Security
		if security == nil {
			security = &spec.Security
		}
		if len(*security) > 0 {
			// Credential validation cannot consume or parse the domain payload.
			authRequest := r.Clone(r.Context())
			authRequest.Body = http.NoBody
			input := &openapi3filter.RequestValidationInput{Request: authRequest, PathParams: pathParams, Route: route, Options: &openapi3filter.Options{AuthenticationFunc: auth}}
			if err := openapi3filter.ValidateSecurityRequirements(r.Context(), input, *security); err != nil {
				code := "AUTHENTICATION_REQUIRED"
				if strings.HasPrefix(r.URL.Path, "/internal/") {
					code = "INTERNAL_AUTHENTICATION_REQUIRED"
				}
				writeError(w, r, 401, code, "Authentication required")
				return
			}
		}
		limit := int64(65536)
		if value, ok := route.Operation.Extensions["x-body-limit"].(float64); ok {
			limit = int64(value)
		}
		if r.Body == nil {
			r.Body = http.NoBody
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
		if err != nil {
			var size *http.MaxBytesError
			if errors.As(err, &size) {
				writeError(w, r, 413, "BODY_TOO_LARGE", "Request body is too large")
			} else {
				writeError(w, r, 400, "MALFORMED_JSON", "Request body could not be read")
			}
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		if route.Operation.RequestBody == nil && len(body) > 0 {
			writeError(w, r, 422, "VALIDATION_FAILED", "Request validation failed", bodyViolation("", "unexpected_body", "Request body is not allowed"))
			return
		}
		if route.Operation.RequestBody != nil {
			if len(body) == 0 {
				writeError(w, r, 422, "VALIDATION_FAILED", "Request validation failed", bodyViolation("", "required", "Request body is required"))
				return
			}
			media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || media != "application/json" {
				writeError(w, r, 415, "UNSUPPORTED_MEDIA_TYPE", "Expected application/json")
				return
			}
			if !json.Valid(body) {
				writeError(w, r, 400, "MALFORMED_JSON", "Malformed JSON")
				return
			}
			var value any
			_ = json.Unmarshal(body, &value)
			mediaSchema := route.Operation.RequestBody.Value.Content["application/json"].Schema.Value
			var violations []violation
			for _, v := range formats.Constraints(mediaSchema, value) {
				violations = append(violations, bodyViolation(v.Pointer, "invalid", v.Message))
			}
			r = r.WithContext(context.WithValue(r.Context(), constraintsKey{}, violations))
		}
		for _, p := range route.Operation.Parameters {
			parameter := p.Value
			if parameter.In != "header" {
				continue
			}
			if len(r.Header.Values(parameter.Name)) > 1 {
				writeError(w, r, 400, "INVALID_HEADER", "Header must occur only once")
				return
			}
		}
		validated.ServeHTTP(w, r)
	})
}

func validationError(spec *openapi3.T, w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, routers.ErrPathNotFound) {
		writeError(w, r, 404, "RESOURCE_NOT_FOUND", "Resource not found")
		return
	}
	if errors.Is(err, routers.ErrMethodNotAllowed) {
		path := spec.Paths.Value(r.URL.Path)
		if path != nil && len(path.Operations()) == 0 {
			writeError(w, r, 404, "RESOURCE_NOT_FOUND", "Resource not found")
			return
		}
		if path != nil {
			methods := make([]string, 0, len(path.Operations()))
			for method := range path.Operations() {
				methods = append(methods, method)
			}
			sort.Strings(methods)
			w.Header().Set("Allow", strings.Join(methods, ", "))
		}
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	violations := collectViolations(err, "body", "")
	if extra, ok := r.Context().Value(constraintsKey{}).([]violation); ok {
		violations = append(violations, extra...)
	}
	for _, v := range violations {
		if v.Location == "header" {
			code, status, message := "INVALID_HEADER", 400, "Invalid header"
			switch v.Parameter {
			case "Idempotency-Key":
				code = "IDEMPOTENCY_KEY_INVALID"
				if r.Header.Get(v.Parameter) == "" {
					code = "IDEMPOTENCY_KEY_REQUIRED"
				}
			// A request without the header carries no allowed origin or token, which the contract
			// rejects with 403. A present but malformed value stays a 400 malformed header.
			case "Origin":
				if r.Header.Get(v.Parameter) == "" {
					code, status, message = "ORIGIN_NOT_ALLOWED", 403, "Origin not allowed"
				}
			case "X-CSRF-Token":
				if r.Header.Get(v.Parameter) == "" {
					code, status, message = "CSRF_INVALID", 403, "Invalid CSRF token"
				}
			}
			writeError(w, r, status, code, message)
			return
		}
		if v.Location == "query" && v.Parameter == "cursor" {
			writeError(w, r, 400, "INVALID_CURSOR", "Invalid cursor")
			return
		}
	}
	writeError(w, r, 422, "VALIDATION_FAILED", "Request validation failed", violations...)
}

func collectViolations(err error, location, parameter string) []violation {
	var result []violation
	switch e := err.(type) {
	case openapi3.MultiError:
		for _, child := range e {
			result = append(result, collectViolations(child, location, parameter)...)
		}
	case *openapi3filter.RequestError:
		if e.Parameter != nil {
			location, parameter = e.Parameter.In, e.Parameter.Name
		}
		result = collectViolations(e.Err, location, parameter)
	case *openapi3.SchemaError:
		if e.Origin != nil {
			if nested := collectViolations(e.Origin, location, parameter); len(nested) > 0 {
				return nested
			}
		}
		code, message := "invalid", "Value does not match the schema"
		if e.SchemaField == "required" {
			code, message = "required", "Required value is missing"
		}
		if e.SchemaField == "properties" {
			code, message = "unknown_field", "Unknown field"
		}
		v := violation{Location: location, Parameter: parameter, Code: code, Message: message}
		if location == "body" {
			parts := e.JSONPointer()
			if e.SchemaField == "properties" {
				quoted := regexp.MustCompile(`^property ("(?:[^"\\]|\\.)*") is unsupported$`).FindStringSubmatch(e.Reason)
				if len(quoted) > 1 {
					if property, err := strconv.Unquote(quoted[1]); err == nil {
						parts = append(parts, property)
					}
				}
			}
			pointer := ""
			for _, part := range parts {
				pointer += "/" + formats.PointerSegment(part)
			}
			v.Pointer = &pointer
		}
		result = append(result, v)
	default:
		v := violation{Location: location, Parameter: parameter, Code: "invalid", Message: "Value does not match the schema"}
		if location == "body" {
			empty := ""
			v.Pointer = &empty
		}
		result = append(result, v)
	}
	return result
}

// writeError emits the single JSON error contract. All four specifications declare the same envelope,
// so the health projection supplies its generated type for every router built on this boundary.
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, violations ...violation) {
	body := healthapi.ApiError{Code: healthapi.ErrorCode(code), Message: message, RequestId: middleware.GetReqID(r.Context())}
	if len(violations) > 0 {
		order := map[string]int{"body": 0, "query": 1, "path": 2, "header": 3}
		key := func(v violation) string {
			if v.Pointer != nil {
				return *v.Pointer
			}
			return v.Parameter
		}
		sort.Slice(violations, func(i, j int) bool {
			a, b := violations[i], violations[j]
			if order[a.Location] != order[b.Location] {
				return order[a.Location] < order[b.Location]
			}
			if key(a) != key(b) {
				return key(a) < key(b)
			}
			return a.Code < b.Code
		})
		unique := violations[:0]
		for _, v := range violations {
			if len(unique) == 0 || unique[len(unique)-1].Location != v.Location || key(unique[len(unique)-1]) != key(v) || unique[len(unique)-1].Code != v.Code {
				unique = append(unique, v)
			}
		}
		data, _ := json.Marshal(struct {
			Violations []violation `json:"violations"`
		}{unique})
		details := healthapi.ApiError_Details{}
		_ = details.UnmarshalJSON(data)
		body.Details = &details
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
