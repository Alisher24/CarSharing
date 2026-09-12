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
	// DatabaseStartupTimeout bounds the wait for the database to accept connections. Compose orders
	// startup, so this only has to cover the first readiness of a cold container.
	DatabaseStartupTimeout = 45 * time.Second

	// pingTimeout bounds one availability probe, so an unreachable host fails that attempt rather
	// than the whole startup deadline.
	pingTimeout = 3 * time.Second

	// connectTimeout is how long the pool gives one connection attempt before reporting the host
	// unreachable. It is the same second-scale bound as a probe, so neither path waits noticeably
	// longer than the other to give up.
	connectTimeout = pingTimeout

	// maxPoolConnections is how many connections this process keeps open at once. The deployment
	// runs one instance beside one PostgreSQL server, so the pool is sized for that.
	maxPoolConnections = 10

	// pingRetryDelay separates two availability probes.
	pingRetryDelay = time.Second

	// sslModeDisableDSN is parsed for its defaults only: host, port, database, user and password are
	// assigned as fields below, never interpolated into a logged URL.
	sslModeDisableDSN = "sslmode=disable"
)

func PoolConfig(cfg config.Config) (*pgxpool.Config, error) {
	poolConfig, err := pgxpool.ParseConfig(sslModeDisableDSN)
	if err != nil {
		return nil, err
	}
	poolConfig.ConnConfig.Host, poolConfig.ConnConfig.Port = cfg.DBHost, cfg.DBPort
	poolConfig.ConnConfig.Database, poolConfig.ConnConfig.User = cfg.DBName, cfg.DBUser
	poolConfig.ConnConfig.Password = cfg.DBPassword
	poolConfig.ConnConfig.ConnectTimeout = connectTimeout
	poolConfig.ConnConfig.RuntimeParams["timezone"] = "UTC"
	poolConfig.MaxConns = maxPoolConnections
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
