package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr   string
	DBHost     string
	DBPort     uint16
	DBName     string
	DBUser     string
	DBPassword string
}

func Load() (Config, error) {
	var c Config
	c.HTTPAddr = value("HTTP_ADDR", ":8080")
	c.DBHost = value("DB_HOST", "postgres")
	c.DBName = value("DB_NAME", "carsharing")
	c.DBUser = value("DB_USER", "carsharing_app")
	port, err := strconv.ParseUint(value("DB_PORT", "5432"), 10, 16)
	if err != nil || port == 0 {
		return c, errors.New("DB_PORT must be between 1 and 65535")
	}
	c.DBPort = uint16(port)
	path := os.Getenv("DB_PASSWORD_FILE")
	if path == "" {
		return c, errors.New("DB_PASSWORD_FILE is required")
	}
	secret, err := os.ReadFile(path)
	if err != nil {
		return c, errors.New("cannot read DB_PASSWORD_FILE")
	}
	c.DBPassword = strings.TrimSpace(string(secret))
	if len(c.DBPassword) < 32 {
		return c, errors.New("database password must contain at least 32 characters; run setup")
	}
	return c, nil
}

func value(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
