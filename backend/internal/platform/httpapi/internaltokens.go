package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	"github.com/getkin/kin-openapi/openapi3filter"
)

// The security schemes the internal contracts declare, one per capability. A scheme name is what the
// boundary hands the authentication function, so the mapping from a capability to the token that
// answers it is stated once, beside the surface that declares it, rather than as a branch at the
// point of use.
const (
	simulatorScheme   = "SimulatorToken"
	demoControlScheme = "DemoToken"
	deliveryScheme    = "DeliveryToken"
	mailDemoScheme    = "MailDemoToken"
)

// bearerPrefix is how the contract's http bearer credential is spelled.
const bearerPrefix = "Bearer "

// errCredentialNotConfigured refuses an operation whose capability this process was given no token
// for. It is a failure of the installation rather than of the request, and it fails closed: an
// internal route without its credential would be a route anybody on the network could call.
var errCredentialNotConfigured = errors.New("this installation was given no token for that capability")

// InternalTokens are the credentials the internal operations of the API accept. Each capability has
// its own token, so a simulator that is allowed to advance the fleet is not thereby allowed to move a
// vehicle by hand, and neither of them is a user's session.
type InternalTokens struct {
	Simulator   string
	DemoControl string
}

// authenticate checks the bearer credential of the operation being served against the token of the
// capabilities pairs each security scheme this surface declares with the token that answers it. The
// table lives beside the surface that declares it, so a new capability is one line here rather than a
// branch in the check every capability shares.
func (t InternalTokens) capabilities() capabilities {
	return capabilities{
		{scheme: simulatorScheme, token: t.Simulator},
		{scheme: demoControlScheme, token: t.DemoControl},
	}
}

// MailstubTokens are the credentials the mail stub's internal operations accept. They are the mail
// stub's own capabilities rather than the API's: the process that stores a letter is not thereby
// allowed to advance the fleet, and the process that sends one carries the delivery token alone.
type MailstubTokens struct {
	Delivery string
	Demo     string
}

// capabilities pairs each security scheme the mail stub declares with the token that answers it.
func (t MailstubTokens) capabilities() capabilities {
	return capabilities{
		{scheme: deliveryScheme, token: t.Delivery},
		{scheme: mailDemoScheme, token: t.Demo},
	}
}

// capability is one security scheme and the token a request is answered by when it presents it.
type capability struct {
	scheme string
	token  string
}

// capabilities is what one internal surface is called with. Both surfaces check a credential the same
// way and refuse an installation that was given none, so each of them says what it accepts in this
// shape and inherits the two answers below.
type capabilities []capability

// authenticate checks the bearer credential of the operation being served against the token of the
// capability that operation names. A missing credential, a malformed one and a wrong one are one
// answer: the boundary reports all three as the same 401, so a caller cannot tell a token that is
// nearly right from one that is absent.
func (c capabilities) authenticate(
	_ context.Context, input *openapi3filter.AuthenticationInput,
) error {
	return authenticateCapability(c.tokenFor(input.SecuritySchemeName), input)
}

// tokenFor is the token that answers one security scheme, or the empty string when this installation
// was given none for it.
func (c capabilities) tokenFor(scheme string) string {
	for _, one := range c {
		if one.scheme == scheme {
			return one.token
		}
	}
	return ""
}

// validate reports which capability this installation cannot serve, or nil when it can serve them all.
func (c capabilities) validate() error {
	for _, required := range c {
		if required.token == "" {
			return errors.New("no token was given for the " + required.scheme + " capability")
		}
	}
	return nil
}

// authenticateCapability compares the bearer credential a request presents with the token one
// capability is answered by. A capability this installation was given no token for fails closed
// rather than being served to anybody who asks.
func authenticateCapability(expected string, input *openapi3filter.AuthenticationInput) error {
	if expected == "" {
		return errCredentialNotConfigured
	}
	presented := strings.TrimPrefix(
		input.RequestValidationInput.Request.Header.Get(authorizationHeader), bearerPrefix)
	if subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) != 1 {
		return errors.New("the presented credential is not one this installation accepts")
	}
	return nil
}
