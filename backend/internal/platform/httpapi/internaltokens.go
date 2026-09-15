package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	"github.com/getkin/kin-openapi/openapi3filter"
)

// The OpenAPI security schemes the internal contract declares, one per capability. A scheme name is
// what the boundary hands the authentication function, so the mapping from a capability to the token
// that answers it is stated once, here, rather than as a branch at the point of use.
const (
	simulatorScheme   = "SimulatorToken"
	demoControlScheme = "DemoToken"
)

// bearerPrefix is how the contract's http bearer credential is spelled.
const bearerPrefix = "Bearer "

// errCredentialNotConfigured refuses an operation whose capability this process was given no token
// for. It is a failure of the installation rather than of the request, and it fails closed: an
// internal route without its credential would be a route anybody on the network could call.
var errCredentialNotConfigured = errors.New("this installation was given no token for that capability")

// InternalTokens are the credentials the internal operations accept. Each capability has its own
// token, so a simulator that is allowed to advance the fleet is not thereby allowed to move a vehicle
// by hand, and neither of them is a user's session.
type InternalTokens struct {
	Simulator   string
	DemoControl string
}

// authenticate checks the bearer credential of the operation being served against the token of the
// capability that operation names. A missing credential, a malformed one and a wrong one are one
// answer: the boundary reports all three as the same 401, so a caller cannot tell a token that is
// nearly right from one that is absent.
func (t InternalTokens) authenticate(
	_ context.Context, input *openapi3filter.AuthenticationInput,
) error {
	expected := t.tokenFor(input.SecuritySchemeName)
	if expected == "" {
		return errCredentialNotConfigured
	}
	presented := strings.TrimPrefix(input.RequestValidationInput.Request.Header.Get(authorizationHeader),
		bearerPrefix)
	if subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) != 1 {
		return errors.New("the presented credential is not one this installation accepts")
	}
	return nil
}

// tokenFor is the token that answers one security scheme, or the empty string when this installation
// was given none for it.
func (t InternalTokens) tokenFor(scheme string) string {
	switch scheme {
	case simulatorScheme:
		return t.Simulator
	case demoControlScheme:
		return t.DemoControl
	default:
		return ""
	}
}

// validate reports which capability this installation cannot serve, or nil when it can serve them all.
func (t InternalTokens) validate() error {
	for _, required := range []struct {
		scheme string
		token  string
	}{
		{simulatorScheme, t.Simulator},
		{demoControlScheme, t.DemoControl},
	} {
		if required.token == "" {
			return errors.New("no token was given for the " + required.scheme + " capability")
		}
	}
	return nil
}
