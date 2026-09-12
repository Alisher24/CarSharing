package sessions

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// store implements the session library's context-aware store interface over PostgreSQL. The
// library's context-free methods answer an error rather than a session, because this application
// always has the context of the request being served and a session that escaped its request must
// not silently succeed.
type store struct{ pool *pgxpool.Pool }

func (s store) FindCtx(ctx context.Context, hashedToken string) ([]byte, bool, error) {
	var data []byte
	err := database.QuerierFrom(ctx, s.pool).QueryRow(ctx,
		`SELECT data FROM sessions WHERE token = $1 AND expiry > now()`, hashedToken).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (s store) CommitCtx(ctx context.Context, hashedToken string, data []byte, expiry time.Time) error {
	_, err := database.QuerierFrom(ctx, s.pool).Exec(ctx,
		`INSERT INTO sessions (token, data, expiry) VALUES ($1, $2, $3)
		 ON CONFLICT (token) DO UPDATE SET data = EXCLUDED.data, expiry = EXCLUDED.expiry`,
		hashedToken, data, expiry)
	return err
}

func (s store) DeleteCtx(ctx context.Context, hashedToken string) error {
	_, err := database.QuerierFrom(ctx, s.pool).Exec(ctx,
		`DELETE FROM sessions WHERE token = $1`, hashedToken)
	return err
}

// errNoRequestContext reports a context-free method of the store, which this application never
// reaches: every statement runs through the context of the request being served.
var errNoRequestContext = errors.New("session store reached without the context of a request")

// The library's interface also declares these context-free methods and prefers the context-aware
// ones when a store provides them.
func (s store) Find(string) ([]byte, bool, error)      { return nil, false, errNoRequestContext }
func (s store) Commit(string, []byte, time.Time) error { return errNoRequestContext }
func (s store) Delete(string) error                    { return errNoRequestContext }
