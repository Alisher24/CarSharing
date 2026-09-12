// Package config reads the process configuration from the environment, taking the database
// password from a file so that a secret never appears in an environment listing.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/ratelimit"
)

// minPasswordLength is the shortest database password setup generates, restated here so a
// hand-edited secret cannot quietly weaken it.
const minPasswordLength = 32

// defaultAllowedOrigins is the documented local profile: the application served over plain HTTP on
// the loopback address under either spelling a browser may use.
const defaultAllowedOrigins = "http://127.0.0.1:8080,http://localhost:8080"

// Starting rate limits from Q11 of the T05 specification. Each is overridable, because the
// values that suit a local demonstration are not the values that suit a deployment.
const (
	defaultSignInEmailAddressAttempts  = 10
	defaultSignInEmailAttempts         = 30
	defaultSignInAddressAttempts       = 100
	defaultRegistrationAddressAttempts = 10
	defaultSignInWindow                = 15 * time.Minute
	defaultRegistrationWindow          = time.Hour
)

// Starting Argon2id cost from Q19 of the T05 specification: 19 MiB of memory, two passes and one
// thread. Each is overridable, because the values that ship are the measured ones.
const (
	defaultArgon2MemoryKiB   = 19 * 1024
	defaultArgon2Passes      = 2
	defaultArgon2Parallelism = 1
	defaultArgon2Concurrent  = 2
)

// Argon2Config is the cost of one password hash. Q19 of the T05 specification fixes the starting
// values; the final ones come from measurements in the target Docker environment, which is why they
// are configuration rather than constants.
type Argon2Config struct {
	MemoryKiB   uint32
	Passes      uint32
	Parallelism uint8

	// Concurrent is how many password hashes one instance computes at a time. Beyond it a request
	// is refused rather than queued, because a queue in front of a memory-hard function is how the
	// instance is made to run out of memory.
	Concurrent int
}

type Config struct {
	HTTPAddr   string
	DBHost     string
	DBPort     uint16
	DBName     string
	DBUser     string
	DBPassword string

	// AllowedOrigins are the browser origins a mutation may come from.
	AllowedOrigins []string

	// SessionCookieSecure adds Secure to the session cookie. It is off only for the documented
	// local HTTP profile; any deployment over HTTPS turns it on.
	SessionCookieSecure bool
	Argon2              Argon2Config
	RateLimits          RateLimitConfig
}

// RateLimitConfig is how many attempts each counted subject may make, and over what window.
type RateLimitConfig struct {
	SignInByEmailAndAddress ratelimit.Limit
	SignInByEmail           ratelimit.Limit
	SignInByAddress         ratelimit.Limit
	RegistrationByAddress   ratelimit.Limit
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
	cfg.AllowedOrigins = splitOrigins(envOrDefault("ALLOWED_ORIGINS", defaultAllowedOrigins))
	cfg.SessionCookieSecure = os.Getenv("SESSION_COOKIE_SECURE") == "true"
	cfg.Argon2, err = loadArgon2()
	if err != nil {
		return cfg, err
	}
	cfg.RateLimits, err = loadRateLimits()
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}

func loadRateLimits() (RateLimitConfig, error) {
	var limits RateLimitConfig
	var err error
	for _, setting := range []struct {
		key      string
		attempts uint64
		window   time.Duration
		assign   func(ratelimit.Limit)
	}{
		{"SIGNIN_EMAIL_ADDRESS", defaultSignInEmailAddressAttempts, defaultSignInWindow,
			func(l ratelimit.Limit) { limits.SignInByEmailAndAddress = l }},
		{"SIGNIN_EMAIL", defaultSignInEmailAttempts, defaultSignInWindow,
			func(l ratelimit.Limit) { limits.SignInByEmail = l }},
		{"SIGNIN_ADDRESS", defaultSignInAddressAttempts, defaultSignInWindow,
			func(l ratelimit.Limit) { limits.SignInByAddress = l }},
		{"REGISTRATION_ADDRESS", defaultRegistrationAddressAttempts, defaultRegistrationWindow,
			func(l ratelimit.Limit) { limits.RegistrationByAddress = l }},
	} {
		var limit ratelimit.Limit
		limit, err = loadLimit(setting.key, setting.attempts, setting.window)
		if err != nil {
			return RateLimitConfig{}, err
		}
		setting.assign(limit)
	}
	return limits, nil
}

// loadLimit reads one limit from RATE_LIMIT_<name>_ATTEMPTS and RATE_LIMIT_<name>_WINDOW.
func loadLimit(name string, attempts uint64, window time.Duration) (ratelimit.Limit, error) {
	counted, err := positiveNumber("RATE_LIMIT_"+name+"_ATTEMPTS", attempts, 31)
	if err != nil {
		return ratelimit.Limit{}, err
	}
	key := "RATE_LIMIT_" + name + "_WINDOW"
	if raw := os.Getenv(key); raw != "" {
		if window, err = time.ParseDuration(raw); err != nil || window <= 0 {
			return ratelimit.Limit{}, errors.New(key + " must be a positive duration such as 15m")
		}
	}
	return ratelimit.Limit{Attempts: int(counted), Window: window}, nil
}

func loadArgon2() (Argon2Config, error) {
	memory, err := positiveNumber("AUTH_ARGON2_MEMORY_KIB", defaultArgon2MemoryKiB, 32)
	if err != nil {
		return Argon2Config{}, err
	}
	passes, err := positiveNumber("AUTH_ARGON2_PASSES", defaultArgon2Passes, 32)
	if err != nil {
		return Argon2Config{}, err
	}
	parallelism, err := positiveNumber("AUTH_ARGON2_PARALLELISM", defaultArgon2Parallelism, 8)
	if err != nil {
		return Argon2Config{}, err
	}
	concurrent, err := positiveNumber("AUTH_ARGON2_CONCURRENT", defaultArgon2Concurrent, 8)
	if err != nil {
		return Argon2Config{}, err
	}
	return Argon2Config{
		MemoryKiB: uint32(memory), Passes: uint32(passes), Parallelism: uint8(parallelism),
		Concurrent: int(concurrent),
	}, nil
}

// positiveNumber reads a setting that must be at least one, so a zero cost cannot be configured by
// leaving a value empty or by mistyping it.
func positiveNumber(key string, fallback uint64, bits int) (uint64, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseUint(raw, 10, bits)
	if err != nil || value == 0 {
		return 0, errors.New(key + " must be a positive number")
	}
	return value, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// splitOrigins reads the comma-separated origin list, ignoring blanks so that a trailing comma or a
// value spread over several lines does not allow the empty origin.
func splitOrigins(raw string) []string {
	var origins []string
	for _, origin := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}
