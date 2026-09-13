package httpapi

import (
	"strings"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
)

// internalPathPrefix is where the internal API's operations live, which is what tells an internal
// authentication failure apart from a public one.
const internalPathPrefix = "/internal/"

// originSet indexes the allowed origins for lookup, so the check is a comparison rather than a scan.
func originSet(origins []string) map[string]bool {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[origin] = true
	}
	return allowed
}

// requireAllowedOrigin refuses a browser mutation that did not come from this application. The
// contract names the operations that check it by declaring a required Origin parameter, so this
// step follows the contract instead of a list of paths maintained beside it. An absent Origin is
// refused by the same rule as a foreign one: neither is an origin this application allows.
func requireAllowedOrigin(allowed map[string]bool) boundaryStep {
	return func(b *boundaryRequest) *contractError {
		if !declaresHeader(b.route.Operation, originHeader, true) {
			return nil
		}
		if !allowed[b.request.Header.Get(originHeader)] {
			return &contractError{code: codeOriginNotAllowed, message: messageOriginNotAllowed}
		}
		return nil
	}
}

// authenticationCode keeps the internal API's authentication failure distinguishable from a public
// one, so a caller of /internal cannot read it as an expired session.
func authenticationCode(path string) servedapi.ErrorCode {
	if strings.HasPrefix(path, internalPathPrefix) {
		return codeInternalAuthenticationRequired
	}
	return codeAuthenticationRequired
}
