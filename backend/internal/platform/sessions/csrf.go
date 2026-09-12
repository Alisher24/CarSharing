package sessions

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
)

// csrfTokenBytes is the entropy of one CSRF token. It is drawn from the same source as the session
// token, because a guessable value would let a foreign page act with the session it protects.
const csrfTokenBytes = 32

func newCSRFToken() (string, error) {
	value := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(value); err != nil {
		return "", errors.New("cannot draw a CSRF token")
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
