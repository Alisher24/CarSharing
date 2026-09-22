package database_test

import (
	"context"
	"testing"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPoolCanBeAssembledFromExplicitSettings(t *testing.T) {
	settings := database.Settings{
		Host:     "database.invalid",
		Port:     5433,
		Name:     "test_records",
		User:     "test_reader",
		Password: "test-only-password",
	}
	configuration, err := database.PoolConfig(settings)
	if err != nil {
		t.Fatal(err)
	}
	connection := configuration.ConnConfig
	if connection.Host != settings.Host || connection.Port != settings.Port ||
		connection.Database != settings.Name || connection.User != settings.User ||
		connection.Password != settings.Password {
		t.Fatal("the connection did not use the supplied settings")
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
}
