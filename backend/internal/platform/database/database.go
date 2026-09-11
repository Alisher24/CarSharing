package database

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func PoolConfig(c config.Config) (*pgxpool.Config, error) {
	// Credentials are assigned as fields, never interpolated into a logged URL.
	p, err := pgxpool.ParseConfig("sslmode=disable")
	if err != nil {
		return nil, err
	}
	p.ConnConfig.Host, p.ConnConfig.Port = c.DBHost, c.DBPort
	p.ConnConfig.Database, p.ConnConfig.User = c.DBName, c.DBUser
	p.ConnConfig.Password = c.DBPassword
	p.ConnConfig.ConnectTimeout = 3 * time.Second
	p.ConnConfig.RuntimeParams["timezone"] = "UTC"
	p.MaxConns = 10
	return p, nil
}

func Open(ctx context.Context, c config.Config) (*pgxpool.Pool, error) {
	pc, err := PoolConfig(c)
	if err != nil {
		return nil, err
	}
	p, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, errors.New("cannot configure database pool")
	}
	// Compose orders startup; bounded retries also support direct process startup.
	for {
		attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = p.Ping(attempt)
		cancel()
		if err == nil {
			return p, nil
		}
		select {
		case <-ctx.Done():
			p.Close()
			return nil, errors.New("database did not become available before startup deadline")
		case <-time.After(time.Second):
		}
	}
}
