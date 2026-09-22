package auth

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors the store reports about an account rather than about the database.
var (
	// ErrEmailTaken reports that the canonical email already identifies an account. Its text is the
	// one a registration is answered with, so the refusal is published rather than restated.
	ErrEmailTaken = errors.New("Email is already registered")

	// ErrUserNotFound reports that no account carries the requested identity. Callers that answer
	// a sign-in must not let it reach the client on its own: an unknown email and a wrong password
	// are answered identically.
	ErrUserNotFound = errors.New("user not found")
)

// User is an account as the contract publishes it: an identity, its canonical email and when it
// was created. The password hash is deliberately not a field, so a value of this type can be
// serialized into a response without leaking one.
type User struct {
	ID        uuid.UUID
	Email     Email
	CreatedAt time.Time
}

// UserStore reads and writes accounts. Every statement runs on the querier the context carries, so
// a call made inside a transaction commits with it and a rollback leaves no account behind.
type UserStore struct{ pool *pgxpool.Pool }

func NewUserStore(pool *pgxpool.Pool) *UserStore { return &UserStore{pool: pool} }

// Create inserts an account and reports ErrEmailTaken when the canonical email is already held.
// The conflict is resolved by the database rather than by a preceding read, so two registrations
// racing for one email create exactly one account.
func (s *UserStore) Create(ctx context.Context, email Email, passwordHash string) (User, error) {
	user := User{Email: email}
	id, err := uuid.NewV7()
	if err != nil {
		return User{}, err
	}
	err = database.QuerierFrom(ctx, s.pool).QueryRow(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)
		 ON CONFLICT (email) DO NOTHING
		 RETURNING id, created_at`, id, string(email), passwordHash).Scan(&user.ID, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrEmailTaken
	}
	if err != nil {
		return User{}, err
	}
	return user, nil
}

// ByEmail returns the account holding a canonical email together with its stored password hash.
func (s *UserStore) ByEmail(ctx context.Context, email Email) (User, string, error) {
	user := User{Email: email}
	var passwordHash string
	err := database.QuerierFrom(ctx, s.pool).QueryRow(ctx,
		`SELECT id, created_at, password_hash FROM users WHERE email = $1`,
		string(email)).Scan(&user.ID, &user.CreatedAt, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrUserNotFound
	}
	if err != nil {
		return User{}, "", err
	}
	return user, passwordHash, nil
}

// ByID returns the account a live session belongs to.
func (s *UserStore) ByID(ctx context.Context, userID uuid.UUID) (User, error) {
	user := User{ID: userID}
	var email string
	err := database.QuerierFrom(ctx, s.pool).QueryRow(ctx,
		`SELECT email, created_at FROM users WHERE id = $1`, userID).Scan(&email, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, err
	}
	user.Email = Email(email)
	return user, nil
}
