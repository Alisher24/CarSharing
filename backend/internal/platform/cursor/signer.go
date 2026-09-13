package cursor

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/google/uuid"
)

// The failures a cursor can carry. The package reports which one it met and leaves the status it
// becomes to the transport: every one of them is the contract's INVALID_CURSOR, whose whole point is
// that a client learns nothing about why its cursor was refused.
var (
	// ErrMalformed reports a value that is not a cursor this package produced.
	ErrMalformed = errors.New("the cursor is malformed")

	// ErrSignature reports a cursor whose signature does not match its payload, which is what
	// tampering with a cursor produces.
	ErrSignature = errors.New("the cursor signature does not match")

	// ErrScope reports a cursor issued for another operation or another owner than the one it was
	// presented to.
	ErrScope = errors.New("the cursor belongs to another collection")
)

// version is the layout of the payload this package writes. A cursor carrying another one was
// produced by a build that spelled its position differently and is refused rather than guessed at.
const version = 1

// The pieces of a token and the limits a scope is held to. A token is one segment of the alphabet
// the contract declares for a cursor, so the signature cannot be told from the payload by a client
// and neither can be edited piecewise.
const (
	scopeSeparator  = "\x00"
	maxIdentifier   = 128
	maxOperation    = 64
	maxParameterKey = 64
	maxParameterVal = 256
)

// signatureLength is how many characters a signature occupies in the token's alphabet: SHA-256
// without padding, which is 43 for a 32-byte hash. It is stated rather than derived, because the
// length of a token is what says where its signature ends.
var signatureLength = base64.RawURLEncoding.EncodedLen(sha256.Size)

// Position is where a page starts: the sort key of its last item. Both parts are read from the stored
// record rather than from the answer, so the page after a cursor continues exactly after the item
// the cursor was taken from.
type Position struct {
	// CreatedAt is the moment the last item of the previous page was created.
	CreatedAt time.Time

	// ID is that item's identifier, which breaks ties between items created at the same moment.
	ID string
}

// Scope is what a cursor is issued for: the operation that issued it, the account it belongs to, and
// the query parameters it was issued under. It is built by OperationOn rather than assembled, so the
// parameters a cursor binds are the ones the operation declares.
type Scope struct {
	operation string
	owner     uuid.UUID
	params    []Parameter
}

// OperationOn names the operation and the account a cursor belongs to. A cursor issued for one
// operation or one account is refused by another, and a newly declared parameter joins the scope by
// being passed here rather than by a second call that could be forgotten.
func OperationOn(operation string, owner uuid.UUID, params ...Parameter) Scope {
	return Scope{operation: operation, owner: owner, params: params}
}

// Parameter pairs the name of a query parameter with the value a page was read under.
type Parameter struct {
	Name  string
	Value string
}

// Signer issues and reads the cursors of one installation. It holds the one signing key that
// configuration read at startup: no default key exists, and a rotation of the key invalidates every
// cursor issued before it.
type Signer struct{ key []byte }

// MinKeyLength is the shortest signing key this package accepts. A key shorter than a hash block
// leaves the signature weaker than the algorithm it names, so a truncated secret is refused where
// the configuration is read rather than served.
const MinKeyLength = 32

// NewSigner returns the signer of one installation. A key too short to be a secret refuses startup
// with a message naming the requirement.
func NewSigner(key []byte) (*Signer, error) {
	if len(key) < MinKeyLength {
		return nil, fmt.Errorf("the cursor signing key must be at least %d bytes", MinKeyLength)
	}
	return &Signer{key: append([]byte(nil), key...)}, nil
}

// Issue returns the token that reads the page after a position within one scope. The token is its
// signature followed by the payload it covers, in one segment of the alphabet the contract declares
// for a cursor: the two cannot be told apart by a client, and neither can be edited piecewise.
func (s *Signer) Issue(position Position, scope Scope) (string, error) {
	if s == nil {
		return "", errors.New("a cursor was issued without a signer")
	}
	payload, err := s.payload(position, scope)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return s.signature(encoded) + encoded, nil
}

// Read returns the position a token carries, having checked that it was issued for exactly the scope
// it is presented to: the same operation, the same account and the same parameters. Every failure —
// a token that is not one of ours, a signature that does not match its payload, a payload that does
// not describe a position — is reported as it is and answered by the caller as the one refusal the
// contract declares.
func (s *Signer) Read(token string, expected Scope) (Position, error) {
	if s == nil {
		return Position{}, ErrMalformed
	}
	presented, encoded, ok := splitToken(token)
	if !ok {
		return Position{}, ErrMalformed
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return Position{}, ErrMalformed
	}
	// The payload is verified before it is read: a value that was not signed by this key never
	// reaches the parser, so a tampered cursor cannot be refused for the wrong reason.
	if !hmac.Equal([]byte(presented), []byte(s.signature(encoded))) {
		return Position{}, ErrSignature
	}
	position, carried, err := parsePayload(payload)
	if err != nil {
		return Position{}, err
	}
	if !carried.matchesScope(expected) {
		return Position{}, ErrScope
	}
	return position, nil
}

// splitToken reports the signature and the payload of a token, and whether the value is a token this
// package wrote at all. A token carries no separator: the signature is the first signatureLength
// characters, and a value too short to hold one, or one whose signature is not the alphabet, is not
// a cursor.
func splitToken(token string) (signature, encoded string, ok bool) {
	if len(token) <= signatureLength {
		return "", "", false
	}
	signature = token[:signatureLength]
	if _, err := base64.RawURLEncoding.Strict().DecodeString(signature); err != nil {
		return "", "", false
	}
	return signature, token[signatureLength:], true
}

// payload spells the position and the scope a token carries. The creation moment is rendered in the
// contract's own format, so a cursor names the moment the collection published rather than a second
// spelling of it that could round differently.
func (s *Signer) payload(position Position, scope Scope) ([]byte, error) {
	if err := scope.validate(); err != nil {
		return nil, err
	}
	if position.CreatedAt.IsZero() {
		return nil, errors.New("a cursor needs the moment of the position it reads after")
	}
	if err := validIdentifier("a cursor position identifier", position.ID); err != nil {
		return nil, err
	}
	parameters := make(map[string]string, len(scope.params))
	for _, one := range scope.params {
		parameters[one.Name] = one.Value
	}
	return json.Marshal(payload{
		Version:   version,
		CreatedAt: timestamp.Format(position.CreatedAt),
		ID:        position.ID,
		Operation: scope.operation,
		Owner:     scope.owner.String(),
		Params:    parameters,
	})
}

// payload is the shape a token carries. It is a private type: the transport never assembles one, so
// the fields of a cursor cannot drift from the ones the signature covers.
type payload struct {
	Version   int               `json:"v"`
	CreatedAt string            `json:"t"`
	ID        string            `json:"id"`
	Operation string            `json:"op"`
	Owner     string            `json:"sub"`
	Params    map[string]string `json:"q,omitempty"`
}

// signature is the HMAC-SHA256 of the encoded payload, rendered in the alphabet the contract's
// cursor declares. It covers the payload verbatim rather than a value rebuilt from it, so any change
// to the bytes a client presents is a change the signature does not match.
func (s *Signer) signature(encodedPayload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(encodedPayload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// parsePayload reads a payload this signer has already verified.
func parsePayload(payload []byte) (Position, Scope, error) {
	var carried struct {
		Version   int               `json:"v"`
		CreatedAt string            `json:"t"`
		ID        string            `json:"id"`
		Operation string            `json:"op"`
		Owner     string            `json:"sub"`
		Params    map[string]string `json:"q"`
	}
	if err := json.Unmarshal(payload, &carried); err != nil {
		return Position{}, Scope{}, ErrMalformed
	}
	if carried.Version != version {
		return Position{}, Scope{}, ErrMalformed
	}
	moment, err := time.Parse(timestamp.Layout, carried.CreatedAt)
	if err != nil {
		return Position{}, Scope{}, ErrMalformed
	}
	owner, err := uuid.Parse(carried.Owner)
	if err != nil {
		return Position{}, Scope{}, ErrMalformed
	}
	params := make([]Parameter, 0, len(carried.Params))
	names := make([]string, 0, len(carried.Params))
	for name := range carried.Params {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		params = append(params, Parameter{Name: name, Value: carried.Params[name]})
	}
	scope := OperationOn(carried.Operation, owner, params...)
	if err := scope.validate(); err != nil {
		return Position{}, Scope{}, ErrMalformed
	}
	if err := validIdentifier("a cursor position identifier", carried.ID); err != nil {
		return Position{}, Scope{}, ErrMalformed
	}
	return Position{CreatedAt: moment, ID: carried.ID}, scope, nil
}

// matchesScope reports whether a cursor was issued for exactly the scope it is presented to: the
// same operation, the same account and the same parameters. A cursor is refused here rather than
// silently read from a position the caller did not ask for.
func (s Scope) matchesScope(expected Scope) bool {
	return s.scopeText() == expected.scopeText()
}

// scopeText is the one spelling of a scope. Its parts are length-prefixed so that no parameter name
// or value can be read as a separator and make two different scopes spell alike.
func (s Scope) scopeText() string {
	var text strings.Builder
	field(&text, s.operation)
	field(&text, s.owner.String())
	for _, one := range s.params {
		field(&text, one.Name)
		field(&text, one.Value)
	}
	return text.String()
}

func field(text *strings.Builder, value string) {
	text.WriteString(strconv.Itoa(len(value)))
	text.WriteString(scopeSeparator)
	text.WriteString(value)
}

// validate refuses a scope that cannot be signed, which is a defect of the operation that built it
// rather than something a client can cause.
func (s Scope) validate() error {
	if s.operation == "" {
		return errors.New("a cursor needs the operation it belongs to")
	}
	if len(s.operation) > maxOperation {
		return errors.New("the operation name of a cursor is too long")
	}
	if s.owner == uuid.Nil {
		return errors.New("a cursor needs the account it belongs to")
	}
	seen := make(map[string]bool, len(s.params))
	for _, one := range s.params {
		if one.Name == "" || len(one.Name) > maxParameterKey {
			return errors.New("a parameter of a cursor needs a name")
		}
		if len(one.Value) > maxParameterVal || !utf8.ValidString(one.Value) {
			return errors.New("the value of a cursor parameter is too long")
		}
		if seen[one.Name] {
			return errors.New("a cursor parameter is stated twice")
		}
		seen[one.Name] = true
	}
	return nil
}

func validIdentifier(what, value string) error {
	if value == "" {
		return fmt.Errorf("%s is missing", what)
	}
	if len(value) > maxIdentifier {
		return fmt.Errorf("%s is too long", what)
	}
	return nil
}
