package httpapi

import (
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
)

// Transport error codes the boundary can answer with, which is what errorStatus below maps to a
// status. A code that only a generated response type carries needs no alias here, and the account
// operations name the generated constant directly. Every specification declares the same error
// enum, so the health projection's generated constants are the single source for their spelling.
const (
	codeAuthenticationRequired         = servedapi.AUTHENTICATIONREQUIRED
	codeInvalidCredentials             = servedapi.INVALIDCREDENTIALS
	codeRateLimited                    = servedapi.RATELIMITED
	codeBodyTooLarge                   = servedapi.BODYTOOLARGE
	codeCSRFInvalid                    = servedapi.CSRFINVALID
	codeIdempotencyKeyInvalid          = servedapi.IDEMPOTENCYKEYINVALID
	codeIdempotencyKeyRequired         = servedapi.IDEMPOTENCYKEYREQUIRED
	codeInternalAuthenticationRequired = servedapi.INTERNALAUTHENTICATIONREQUIRED
	codeInternalError                  = servedapi.INTERNALERROR
	codeInvalidCursor                  = servedapi.INVALIDCURSOR
	codeInvalidHeader                  = servedapi.INVALIDHEADER
	codeMalformedJSON                  = servedapi.MALFORMEDJSON
	codeMethodNotAllowed               = servedapi.METHODNOTALLOWED
	codeOriginNotAllowed               = servedapi.ORIGINNOTALLOWED
	codeResourceNotFound               = servedapi.RESOURCENOTFOUND
	codeServiceUnavailable             = servedapi.SERVICEUNAVAILABLE
	codeUnsupportedMediaType           = servedapi.UNSUPPORTEDMEDIATYPE
	codeValidationFailed               = servedapi.VALIDATIONFAILED
)

// Messages shared by more than one failure. A message specific to a single failure is spelled at
// the point of use instead.
const (
	messageAuthenticationRequired = "Authentication required"
	messageCSRFInvalid            = "Invalid CSRF token"
	messageInvalidCursor          = "Invalid cursor"
	messageOriginNotAllowed       = "Origin not allowed"
	messageInternalError          = "Internal server error"
	messageInvalidHeader          = "Invalid header"
	messageMalformedJSON          = "Malformed JSON"
	messageMethodNotAllowed       = "Method not allowed"
	messageResourceNotFound       = "Resource not found"
	messageServiceUnavailable     = "Service unavailable"
	messageValidationFailed       = "Request validation failed"
)
