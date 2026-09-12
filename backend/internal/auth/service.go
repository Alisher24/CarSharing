package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"
)

// standInSecretBytes is the entropy of the per-process value the stand-in hash is derived from.
const standInSecretBytes = 32

// ErrInvalidCredentials reports a sign-in that cannot be granted. An unknown email and a wrong
// password both produce it, so a caller cannot learn from the answer whether an address is
// registered.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Service creates accounts and proves them. It owns the order the rules are applied in: the
// credentials are shaped before any account is touched, and a password is hashed only once the
// rest of the request is known to be worth the work.
type Service struct {
	users  *UserStore
	hasher *PasswordHasher

	// standInHash is verified against when no account holds the address, so that the memory-hard
	// work an unknown address costs matches what a known one costs. It is derived once at startup
	// from a value no account can hold, and is never a password anyone could present.
	standInHash string
}

func NewService(users *UserStore, hasher *PasswordHasher) (*Service, error) {
	standInHash, err := hasher.Hash(standInSecret())
	if err != nil {
		return nil, err
	}
	return &Service{users: users, hasher: hasher, standInHash: standInHash}, nil
}

// standInSecret is a fresh random value per process. Nothing verifies against it successfully, and
// drawing it rather than fixing it keeps a constant out of the sources that could be mistaken for
// a credential.
func standInSecret() string {
	value := make([]byte, standInSecretBytes)
	if _, err := rand.Read(value); err != nil {
		// A process that cannot read randomness cannot hash passwords either; the caller learns
		// that from the hashing failure this produces rather than from a silent weak value.
		return time.Now().UTC().String()
	}
	return base64.RawURLEncoding.EncodeToString(value)
}

// Register creates an account for a canonical email. It reports ErrEmailTaken when the address is
// already held. The caller runs it inside the transaction that also stores the new session, so a
// failure anywhere in that transaction leaves no account behind.
func (s *Service) Register(ctx context.Context, email Email, password string) (User, error) {
	if err := ValidatePassword(password); err != nil {
		return User{}, err
	}
	passwordHash, err := s.hasher.Hash(password)
	if err != nil {
		return User{}, err
	}
	return s.users.Create(ctx, email, passwordHash)
}

// Authenticate proves a password against the account holding a canonical email, reporting
// ErrInvalidCredentials for both an unknown address and a wrong password.
//
// An unknown address is verified against a stand-in hash rather than refused straight away.
// Argon2id is the slowest part of a sign-in, so skipping it would make an unregistered address
// answer visibly faster than a registered one and turn the sign-in form into a way of asking which
// addresses have accounts. This equalizes the memory-hard work, not the whole response time.
func (s *Service) Authenticate(ctx context.Context, email Email, password string) (User, error) {
	user, passwordHash, err := s.users.ByEmail(ctx, email)
	if errors.Is(err, ErrUserNotFound) {
		if _, verifyErr := s.hasher.Verify(s.standInHash, password); verifyErr != nil {
			return User{}, verifyErr
		}
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, err
	}
	correct, err := s.hasher.Verify(passwordHash, password)
	if err != nil {
		return User{}, err
	}
	if !correct {
		return User{}, ErrInvalidCredentials
	}
	return user, nil
}
