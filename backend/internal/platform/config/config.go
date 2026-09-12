// Package config reads the process configuration from the environment, taking the database
// password from a file so that a secret never appears in an environment listing.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// minPasswordLength is the shortest database password setup generates, restated here so a
// hand-edited secret cannot quietly weaken it.
const minPasswordLength = 32

type Config struct {
	HTTPAddr   string
	DBHost     string
	DBPort     uint16
	DBName     string
	DBUser     string
	DBPassword string
}

func Load() (Config, error) {
	var cfg Config
	cfg.HTTPAddr = envOrDefault("HTTP_ADDR", ":8080")
	cfg.DBHost = envOrDefault("DB_HOST", "postgres")
	cfg.DBName = envOrDefault("DB_NAME", "carsharing")
	cfg.DBUser = envOrDefault("DB_USER", "carsharing_app")
	port, err := strconv.ParseUint(envOrDefault("DB_PORT", "5432"), 10, 16)
	if err != nil || port == 0 {
		return cfg, errors.New("DB_PORT must be between 1 and 65535")
	}
	cfg.DBPort = uint16(port)
	path := os.Getenv("DB_PASSWORD_FILE")
	if path == "" {
		return cfg, errors.New("DB_PASSWORD_FILE is required")
	}
	secret, err := os.ReadFile(path)
	if err != nil {
		return cfg, errors.New("cannot read DB_PASSWORD_FILE")
	}
	cfg.DBPassword = strings.TrimSpace(string(secret))
	if len(cfg.DBPassword) < minPasswordLength {
		return cfg, errors.New("database password must contain at least 32 characters; run setup")
	}
	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
