package httpapi

import (
	"net/http"
)

// writeValidationError reports an invalid payload or parameter. Positional constraints were
// collected before the validator ran, so they are merged in here to report one complete set of
// violations. A parameter the contract gives its own code is answered with that code instead.
func writeValidationError(w http.ResponseWriter, r *http.Request, err error) {
	violations := collectViolations(err, locationBody, "")
	if carried, ok := r.Context().Value(constraintsKey{}).([]violation); ok {
		violations = append(violations, carried...)
	}
	for _, failed := range violations {
		if failure := parameterFailure(r, failed); failure != nil {
			writeError(w, r, failure.code, failure.message)
			return
		}
	}
	writeError(w, r, codeValidationFailed, messageValidationFailed, violations...)
}

// parameterFailure maps a violation on a parameter the contract answers with a dedicated code, and
// reports nil when the parameter is covered by the generic validation response.
func parameterFailure(r *http.Request, failed violation) *contractError {
	switch {
	case failed.Location == locationHeader:
		return headerFailure(r, failed.Parameter)
	case failed.Location == locationQuery && failed.Parameter == cursorParameter:
		return &contractError{code: codeInvalidCursor, message: messageInvalidCursor}
	}
	return nil
}

// headerFailure separates an absent header from a malformed one. A request without Origin or
// X-CSRF-Token carries no allowed origin or token at all, which the contract rejects with 403,
// while a header that is present but malformed stays a malformed header.
func headerFailure(r *http.Request, name string) *contractError {
	absent := r.Header.Get(name) == ""
	switch name {
	case idempotencyKeyHeader:
		if absent {
			return &contractError{code: codeIdempotencyKeyRequired, message: messageInvalidHeader}
		}
		return &contractError{code: codeIdempotencyKeyInvalid, message: messageInvalidHeader}
	case originHeader:
		if absent {
			return &contractError{code: codeOriginNotAllowed, message: messageOriginNotAllowed}
		}
	case csrfTokenHeader:
		if absent {
			return &contractError{code: codeCSRFInvalid, message: messageCSRFInvalid}
		}
	}
	return &contractError{code: codeInvalidHeader, message: messageInvalidHeader}
}
