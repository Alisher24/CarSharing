// Package config reads the process configuration from the environment, taking the database
// password from a file so that a secret never appears in an environment listing.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/hashing"
	"github.com/Alisher24/CarSharing/backend/internal/platform/ratelimit"
)

// The profile a process is started as. Only the demo profile seeds data a deployment must not
// carry, so the command that writes it refuses every other value.
const (
	ProductionEnvironment = "production"
	DemoEnvironment       = "demo"
)

// The setting that names the listener, the address it defaults to, and the reader a process uses
// when it needs only this setting — the container's own health check, which cannot load the rest of
// the configuration because it is given no database secret.
const (
	HTTPAddrVariable = "HTTP_ADDR"
	DefaultHTTPAddr  = ":8080"
)

// HTTPAddrFromEnvironment reports the listener the API was started with, applying the default when
// the setting is absent.
func HTTPAddrFromEnvironment() string {
	if value := os.Getenv(HTTPAddrVariable); value != "" {
		return value
	}
	return DefaultHTTPAddr
}

// DatabasePasswordFileVariable names the file holding the password the process connects to the
// database with. Every process is given one; none of them is given the password itself.
const DatabasePasswordFileVariable = "DB_PASSWORD_FILE"

// DemoUserPasswordFileVariable names the file holding the password the two demonstration accounts
// a person signs in as are created with. Only the command that installs the demonstration is given
// it, so nothing else can create an account somebody could sign in as.
const DemoUserPasswordFileVariable = "DEMO_USER_PASSWORD_FILE"

// minPasswordLength is the shortest database password setup generates, restated here so a
// hand-edited secret cannot quietly weaken it. Its length is the one the failure below states.
const minPasswordLength = 32

// minPasswordLengthMessage refuses a secret shorter than minPasswordLength. It names the length so
// that the person reading the failure knows what setup would have produced.
const minPasswordLengthMessage = "database password must contain at least " +
	"32 characters; run setup"

// defaultAllowedOrigins is the documented local profile: the application served over plain HTTP on
// the loopback address under either spelling a browser may use.
const defaultAllowedOrigins = "http://127.0.0.1:8080,http://localhost:8080"

// Starting rate limits. Each is overridable, because the values that suit a local demonstration
// are not the values that suit a deployment.
const (
	defaultSignInEmailAddressAttempts  = 10
	defaultSignInEmailAttempts         = 30
	defaultSignInAddressAttempts       = 100
	defaultRegistrationAddressAttempts = 10
	defaultSignInWindow                = 15 * time.Minute
	defaultRegistrationWindow          = time.Hour
)

// Starting Argon2id cost: 19 MiB of memory, two passes and one thread. Each is overridable, because
// the values that ship are the measured ones.
const (
	defaultArgon2MemoryKiB   = 19 * 1024
	defaultArgon2Passes      = 2
	defaultArgon2Parallelism = 1
	defaultArgon2Concurrent  = 2
)

// Config is everything a process is told about the installation it runs in.
type Config struct {
	HTTPAddr   string
	DBHost     string
	DBPort     uint16
	DBName     string
	DBUser     string
	DBPassword string

	// Environment is what the process was started as: the demo profile adds data a deployment
	// must not seed, so the commands that change stored data read it from here rather than from
	// the environment directly.
	Environment string

	// AllowedOrigins are the browser origins a mutation may come from.
	AllowedOrigins []string

	// SessionCookieSecure adds Secure to the session cookie. It is off only for the documented
	// local HTTP profile; any deployment over HTTPS turns it on.
	SessionCookieSecure bool

	// Argon2 is the cost of one password hash. Both the service and the command that installs the
	// demonstration hash with it, because an account one wrote has to be one the other can verify.
	Argon2 hashing.Cost

	// RateLimits is the budget of each counted account operation.
	RateLimits ratelimit.Limits

	// DemoUserPassword is what the demonstration accounts a person signs in as are created with.
	// It is empty in a process that was not given the file, and the command that needs it refuses
	// to run rather than invent one.
	DemoUserPassword string
}

func Load() (Config, error) {
	var cfg Config
	cfg.HTTPAddr = HTTPAddrFromEnvironment()
	cfg.DBHost = envOrDefault("DB_HOST", "postgres")
	cfg.DBName = envOrDefault("DB_NAME", "carsharing")
	cfg.DBUser = envOrDefault("DB_USER", "carsharing_app")
	cfg.Environment = envOrDefault("APP_ENV", ProductionEnvironment)
	port, err := strconv.ParseUint(envOrDefault("DB_PORT", "5432"), 10, 16)
	if err != nil || port == 0 {
		return cfg, errors.New("DB_PORT must be between 1 and 65535")
	}
	cfg.DBPort = uint16(port)
	if os.Getenv(DatabasePasswordFileVariable) == "" {
		return cfg, errors.New(DatabasePasswordFileVariable + " is required")
	}
	cfg.DBPassword, err = secretFromFile(DatabasePasswordFileVariable)
	if err != nil {
		return cfg, err
	}
	if len(cfg.DBPassword) < minPasswordLength {
		return cfg, errors.New(minPasswordLengthMessage)
	}
	cfg.DemoUserPassword, err = secretFromFile(DemoUserPasswordFileVariable)
	if err != nil {
		return cfg, err
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

// rateLimitSetting names one counted limit: the settings it is read from, the values it starts at,
// and where the result belongs in the configuration.
type rateLimitSetting struct {
	name     string
	attempts uint64
	window   time.Duration
	assign   func(*ratelimit.Limits, ratelimit.Limit)
}

func loadRateLimits() (ratelimit.Limits, error) {
	settings := []rateLimitSetting{
		{"SIGNIN_EMAIL_ADDRESS", defaultSignInEmailAddressAttempts, defaultSignInWindow,
			func(limits *ratelimit.Limits, limit ratelimit.Limit) { limits.SignInByEmailAndAddress = limit }},
		{"SIGNIN_EMAIL", defaultSignInEmailAttempts, defaultSignInWindow,
			func(limits *ratelimit.Limits, limit ratelimit.Limit) { limits.SignInByEmail = limit }},
		{"SIGNIN_ADDRESS", defaultSignInAddressAttempts, defaultSignInWindow,
			func(limits *ratelimit.Limits, limit ratelimit.Limit) { limits.SignInByAddress = limit }},
		{"REGISTRATION_ADDRESS", defaultRegistrationAddressAttempts, defaultRegistrationWindow,
			func(limits *ratelimit.Limits, limit ratelimit.Limit) { limits.RegistrationByAddress = limit }},
	}

	var limits ratelimit.Limits
	for _, setting := range settings {
		limit, err := loadLimit(setting.name, setting.attempts, setting.window)
		if err != nil {
			return ratelimit.Limits{}, err
		}
		setting.assign(&limits, limit)
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

func loadArgon2() (hashing.Cost, error) {
	memory, err := positiveNumber("AUTH_ARGON2_MEMORY_KIB", defaultArgon2MemoryKiB, 32)
	if err != nil {
		return hashing.Cost{}, err
	}
	passes, err := positiveNumber("AUTH_ARGON2_PASSES", defaultArgon2Passes, 32)
	if err != nil {
		return hashing.Cost{}, err
	}
	parallelism, err := positiveNumber("AUTH_ARGON2_PARALLELISM", defaultArgon2Parallelism, 8)
	if err != nil {
		return hashing.Cost{}, err
	}
	concurrent, err := positiveNumber("AUTH_ARGON2_CONCURRENT", defaultArgon2Concurrent, 8)
	if err != nil {
		return hashing.Cost{}, err
	}
	return hashing.Cost{
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

// secretFromFile reads the secret a setting points at. A setting that names no file yields no
// secret, because a process is given only the secrets it needs; a setting that names a file that
// cannot be read is a misconfiguration and stops the process.
func secretFromFile(key string) (string, error) {
	path := os.Getenv(key)
	if path == "" {
		return "", nil
	}
	secret, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("cannot read " + key)
	}
	return strings.TrimSpace(string(secret)), nil
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
