package auth

import (
	"errors"
	"net/mail"
	"strings"
	"unicode"
)

// maxEmailLength is the longest canonical address the contract accepts, restated here so the
// domain refuses an over-long address that never passed through the HTTP boundary.
const maxEmailLength = 254

// ErrEmailInvalid reports an address that is not a usable email once canonicalized. The caller
// turns it into the contract's validation failure; it never distinguishes which rule was broken,
// because the client is told the field is wrong rather than how to shape a better guess.
var ErrEmailInvalid = errors.New("email is not a valid address")

// Email is an address in canonical form: surrounding whitespace removed and the whole address
// lowercased. Only ParseEmail produces one, so a value of this type is always safe to compare and
// to store as the unique identity of an account.
type Email string

// ParseEmail canonicalizes a client-supplied address and reports whether it is usable. The whole
// address is lowercased, not only the domain: the contract promises one canonical value per
// account, so two spellings of the same address cannot register twice.
func ParseEmail(raw string) (Email, error) {
	canonical := strings.ToLower(strings.TrimSpace(raw))
	if len([]rune(canonical)) > maxEmailLength {
		return "", ErrEmailInvalid
	}
	if !isAddressShape(canonical) {
		return "", ErrEmailInvalid
	}
	address, err := mail.ParseAddress(canonical)
	if err != nil || address.Address != canonical {
		return "", ErrEmailInvalid
	}
	return Email(canonical), nil
}

// isAddressShape applies the constraints net/mail does not: it accepts a display name, an address
// without a dotted domain and an address carrying internal whitespace, none of which the contract
// permits.
func isAddressShape(canonical string) bool {
	localPart, domain, found := strings.Cut(canonical, "@")
	if !found || localPart == "" || domain == "" {
		return false
	}
	if strings.ContainsRune(domain, '@') || !strings.Contains(domain, ".") {
		return false
	}
	return strings.IndexFunc(canonical, unicode.IsSpace) < 0
}
