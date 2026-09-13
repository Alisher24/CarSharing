package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
)

// Key is the identifier a client gives one command. The contract fixes its spelling as a canonical
// unquoted UUID v4, which the validation boundary has already checked by the time a handler reads
// it, and scopes it to the account that sent it.
type Key string

// Fingerprint is what a command asks for, reduced to one value. It covers the method, the path with
// the identifier of the resource it names, and the canonical body, so that the same key applied to
// another method, object or body is a conflict rather than a repeat.
type Fingerprint string

// fingerprintSeparator cannot occur in a method or a path, so two different commands cannot produce
// the same text to hash by moving a character across the boundary between them.
const fingerprintSeparator = "\n"

// FingerprintOf reduces one command to its fingerprint. The body is the canonical form of the
// payload — the contract's own shape for it, as the transport layer re-encoded it — rather than the
// bytes that arrived, so two attempts that mean the same thing fingerprint the same way whatever
// whitespace or field order they were written with.
func FingerprintOf(method, path string, body []byte) Fingerprint {
	digest := sha256.New()
	digest.Write([]byte(method))
	digest.Write([]byte(fingerprintSeparator))
	digest.Write([]byte(path))
	digest.Write([]byte(fingerprintSeparator))
	digest.Write(body)
	return Fingerprint(hex.EncodeToString(digest.Sum(nil)))
}
