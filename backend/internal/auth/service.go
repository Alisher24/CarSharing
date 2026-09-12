package auth

import (
	"context"
	"errors"
)

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
}

func NewService(users *UserStore, hasher *PasswordHasher) *Service {
	return &Service{users: users, hasher: hasher}
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
func (s *Service) Authenticate(ctx context.Context, email Email, password string) (User, error) {
	user, passwordHash, err := s.users.ByEmail(ctx, email)
	if errors.Is(err, ErrUserNotFound) {
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
