package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// encodedHashFields is the field count of the PHC string this package reads and writes:
// an empty leading field, the algorithm, the version, the cost parameters, the salt and the key.
const encodedHashFields = 6

// Sizes of the values a hash is built from. They are not tuning knobs: a 16-byte salt and a 32-byte
// key are what Argon2id is specified against, and shortening either weakens the hash without making
// it cheaper to compute.
const (
	SaltLength = 16
	KeyLength  = 32
)

// ErrStoredHashUnusable reports a stored password hash this build cannot verify against. It is a
// storage defect rather than a wrong password, so a caller answers it as a service failure instead
// of as invalid credentials.
var ErrStoredHashUnusable = errors.New("stored password hash cannot be read")

// HashingParameters is the cost of one Argon2id computation. They are configuration rather than
// constants because the final values come from measurements in the target Docker environment.
type HashingParameters struct {
	MemoryKiB   uint32
	Passes      uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// PasswordHasher derives and checks Argon2id password hashes. Every hash carries its own salt and
// the parameters it was produced with, so raising the cost later does not lock existing accounts
// out.
//
// Argon2id is memory-hard by design, which makes it the most expensive thing a request can ask
// for. The hasher therefore admits only a fixed number of computations at a time and refuses the
// rest outright: a queue in front of a memory-hard function is how one instance is made to exhaust
// its memory and stop answering anything at all.
type PasswordHasher struct {
	parameters HashingParameters
	slots      chan struct{}
}

// ErrHashingBusy reports that every hashing slot on this instance is taken. A caller answers it as
// a service failure, because the work was not attempted rather than attempted and refused.
var ErrHashingBusy = errors.New("no hashing slot is free")

// NewPasswordHasher builds a hasher admitting concurrent computations up to a ceiling. A ceiling
// below one would admit nothing, so it is raised to one.
func NewPasswordHasher(parameters HashingParameters, concurrent int) *PasswordHasher {
	if concurrent < 1 {
		concurrent = 1
	}
	return &PasswordHasher{parameters: parameters, slots: make(chan struct{}, concurrent)}
}

// acquire takes a hashing slot without waiting, so a request that finds the instance saturated is
// told so immediately instead of being parked until something times out.
func (h *PasswordHasher) acquire() bool {
	select {
	case h.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (h *PasswordHasher) release() { <-h.slots }

// Hash derives a new hash under the configured parameters, with a salt drawn for this password
// alone so that two accounts sharing a password do not share a stored value.
func (h *PasswordHasher) Hash(password string) (string, error) {
	if !h.acquire() {
		return "", ErrHashingBusy
	}
	defer h.release()
	salt := make([]byte, h.parameters.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", errors.New("cannot draw a password salt")
	}
	return encodeHash(h.parameters, salt, h.derive(h.parameters, password, salt)), nil
}

// Verify reports whether a password derives the stored hash, using the parameters recorded in that
// hash rather than the configured ones. A wrong password is reported as false; a hash this build
// cannot read is reported as an error, because it must not be mistaken for a failed guess.
func (h *PasswordHasher) Verify(encoded, password string) (bool, error) {
	parameters, salt, storedKey, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	if !h.acquire() {
		return false, ErrHashingBusy
	}
	defer h.release()
	derivedKey := h.derive(parameters, password, salt)
	// Constant time, so the answer does not depend on how much of the hash matched.
	return subtle.ConstantTimeCompare(derivedKey, storedKey) == 1, nil
}

func (h *PasswordHasher) derive(parameters HashingParameters, password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt,
		parameters.Passes, parameters.MemoryKiB, parameters.Parallelism, parameters.KeyLength)
}

func encodeHash(parameters HashingParameters, salt, key []byte) string {
	encoding := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, parameters.MemoryKiB, parameters.Passes, parameters.Parallelism,
		encoding.EncodeToString(salt), encoding.EncodeToString(key))
}

func decodeHash(encoded string) (HashingParameters, []byte, []byte, error) {
	fields := strings.Split(encoded, "$")
	if len(fields) != encodedHashFields || fields[0] != "" || fields[1] != "argon2id" {
		return HashingParameters{}, nil, nil, ErrStoredHashUnusable
	}
	var version int
	if _, err := fmt.Sscanf(fields[2], "v=%d", &version); err != nil || version != argon2.Version {
		return HashingParameters{}, nil, nil, ErrStoredHashUnusable
	}
	var parameters HashingParameters
	read, err := fmt.Sscanf(fields[3], "m=%d,t=%d,p=%d",
		&parameters.MemoryKiB, &parameters.Passes, &parameters.Parallelism)
	if err != nil || read != 3 {
		return HashingParameters{}, nil, nil, ErrStoredHashUnusable
	}
	salt, err := base64.RawStdEncoding.DecodeString(fields[4])
	if err != nil {
		return HashingParameters{}, nil, nil, ErrStoredHashUnusable
	}
	key, err := base64.RawStdEncoding.DecodeString(fields[5])
	if err != nil || len(key) == 0 {
		return HashingParameters{}, nil, nil, ErrStoredHashUnusable
	}
	parameters.SaltLength, parameters.KeyLength = uint32(len(salt)), uint32(len(key))
	return parameters, salt, key, nil
}
