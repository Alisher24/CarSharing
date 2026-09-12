// Package database opens the PostgreSQL pool the application uses, waiting for the server to
// become reachable so that a process started alongside its database survives a cold start.
package database

import (
	"context"
	"errors"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// pingTimeout bounds one availability probe, so an unreachable host fails that attempt rather
	// than the whole startup deadline.
	pingTimeout = 3 * time.Second

	// pingRetryDelay separates two availability probes.
	pingRetryDelay = time.Second
)

func PoolConfig(cfg config.Config) (*pgxpool.Config, error) {
	// Credentials are assigned as fields, never interpolated into a logged URL.
	poolConfig, err := pgxpool.ParseConfig("sslmode=disable")
	if err != nil {
		return nil, err
	}
	poolConfig.ConnConfig.Host, poolConfig.ConnConfig.Port = cfg.DBHost, cfg.DBPort
	poolConfig.ConnConfig.Database, poolConfig.ConnConfig.User = cfg.DBName, cfg.DBUser
	poolConfig.ConnConfig.Password = cfg.DBPassword
	poolConfig.ConnConfig.ConnectTimeout = 3 * time.Second
	poolConfig.ConnConfig.RuntimeParams["timezone"] = "UTC"
	poolConfig.MaxConns = 10
	return poolConfig, nil
}

func Open(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := PoolConfig(cfg)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, errors.New("cannot configure database pool")
	}
	// Compose orders startup; bounded retries also support direct process startup.
	for {
		attempt, cancel := context.WithTimeout(ctx, pingTimeout)
		err = pool.Ping(attempt)
		cancel()
		if err == nil {
			return pool, nil
		}
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, errors.New("database did not become available before startup deadline")
		case <-time.After(pingRetryDelay):
		}
	}
}
