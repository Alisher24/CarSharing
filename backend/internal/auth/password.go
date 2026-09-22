package auth

import (
	"errors"
	"unicode"
	"unicode/utf8"
)

// The range a password must fall in, counted in Unicode code points. The contract declares the
// same bounds, so a password reaching the domain from the seeder rather than from HTTP is held to
// exactly the rule the HTTP boundary applies.
const (
	minPasswordCodePoints = 12
	maxPasswordCodePoints = 128
)

// ErrPasswordInvalid reports a password outside the allowed range or carrying whitespace. Its text is
// the one a registration is answered with. It never says which rule was broken and never carries the
// password, so neither a response nor a log can narrow the search space for a guess.
var ErrPasswordInvalid = errors.New("Password does not meet the policy")

// ValidatePassword reports whether a password may be used. The password is neither trimmed nor
// normalized: what a person typed is what is hashed, so two spellings a normalizer would fold
// together remain distinct secrets.
func ValidatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < minPasswordCodePoints || length > maxPasswordCodePoints {
		return ErrPasswordInvalid
	}
	// Whitespace is refused outright rather than trimmed, because trimming would silently accept a
	// password that differs from the one the person believes they chose.
	if containsWhitespace(password) {
		return ErrPasswordInvalid
	}
	return nil
}

func containsWhitespace(password string) bool {
	for _, character := range password {
		if unicode.IsSpace(character) {
			return true
		}
	}
	return false
}
