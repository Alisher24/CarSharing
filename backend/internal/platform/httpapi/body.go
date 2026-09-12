package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/Alisher24/CarSharing/backend/internal/contracts/formats"
	"github.com/getkin/kin-openapi/openapi3"
)

// defaultBodyLimitBytes is the largest request body an operation accepts unless it declares
// x-body-limit. The internal contracts raise it because a simulation tick carries a batch.
const defaultBodyLimitBytes = 64 << 10

// Messages the body steps answer with.
const (
	messageBodyTooLarge       = "Request body is too large"
	messageBodyUnreadable     = "Request body could not be read"
	messageBodyUnexpected     = "Request body is not allowed"
	messageBodyRequired       = "Request body is required"
	messageContentTypeInvalid = "Expected " + jsonMediaType
)

// bufferBodyWithinLimit reads the body once within the limit of its operation and replaces it with a
// replayable reader, because the schema validator and the handler each read it again.
func bufferBodyWithinLimit(b *boundaryRequest) *contractError {
	if b.request.Body == nil {
		b.request.Body = http.NoBody
	}

	limited := http.MaxBytesReader(b.writer, b.request.Body, bodyLimit(b.route.Operation))
	body, err := io.ReadAll(limited)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return &contractError{code: codeBodyTooLarge, message: messageBodyTooLarge}
		}
		return &contractError{code: codeMalformedJSON, message: messageBodyUnreadable}
	}

	b.body = body
	b.request.Body = io.NopCloser(bytes.NewReader(body))
	return nil
}

func bodyLimit(operation *openapi3.Operation) int64 {
	if declared, ok := operation.Extensions["x-body-limit"].(float64); ok {
		return int64(declared)
	}
	return defaultBodyLimitBytes
}

// requireJSONRequestBody enforces the presence, media type and syntax the operation declares, then
// records the positional constraints OpenAPI 3.0 cannot express so that constraintGate can report
// them once the schema validator has had its say.
func requireJSONRequestBody(b *boundaryRequest) *contractError {
	if b.route.Operation.RequestBody == nil {
		if len(b.body) > 0 {
			return validationFailure(bodyViolation("", codeUnexpectedBody, messageBodyUnexpected))
		}
		return nil
	}

	if len(b.body) == 0 {
		return validationFailure(bodyViolation("", codeRequiredField, messageBodyRequired))
	}

	media, _, err := mime.ParseMediaType(b.request.Header.Get(contentTypeHeader))
	if err != nil || media != jsonMediaType {
		return &contractError{code: codeUnsupportedMediaType, message: messageContentTypeInvalid}
	}

	if !json.Valid(b.body) {
		return &contractError{code: codeMalformedJSON, message: messageMalformedJSON}
	}

	var payload any
	_ = json.Unmarshal(b.body, &payload)
	schema := b.route.Operation.RequestBody.Value.Content[jsonMediaType].Schema.Value
	var violations []violation
	for _, constraint := range formats.Constraints(schema, payload) {
		violations = append(violations, bodyViolation(constraint.Pointer, codeInvalidField, constraint.Message))
	}

	b.request = b.request.WithContext(context.WithValue(b.request.Context(), constraintsKey{}, violations))
	return nil
}

func validationFailure(violations ...violation) *contractError {
	return &contractError{code: codeValidationFailed, message: messageValidationFailed, violations: violations}
}
