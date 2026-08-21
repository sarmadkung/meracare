// Package config loads API configuration from the process environment.
//
// Configuration is read once at startup and validated eagerly so that a
// misconfigured deployment fails immediately instead of at the first request.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment names the deployment environment.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvTest        Environment = "test"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// IsDevelopment reports whether human-readable logging and relaxed defaults apply.
func (e Environment) IsDevelopment() bool { return e == EnvDevelopment || e == EnvTest }

// JWTMode selects how Supabase access tokens are verified.
type JWTMode string

const (
	// JWTModeAsymmetric verifies tokens against the project's published JWKS.
	// The API holds only public keys, so it can verify but never mint a token.
	// This is the default and the recommended Supabase configuration.
	JWTModeAsymmetric JWTMode = "asymmetric"

	// JWTModeLegacyHS256 verifies tokens with the project's shared JWT secret.
	// The same secret both verifies and signs, so anything holding it can forge
	// a token for any user. Only for projects still on the legacy secret.
	JWTModeLegacyHS256 JWTMode = "legacy_hs256"
)

// Config is the fully resolved API configuration.
type Config struct {
	Env                Environment
	Port               int
	LogLevel           string
	CORSAllowedOrigins []string

	DatabaseURL      string
	DatabaseMaxConns int32

	SupabaseURL         string
	SupabaseJWTMode     JWTMode
	SupabaseJWTSecret   string
	SupabaseJWTAudience string
	SupabaseJWTLeeway   time.Duration
	ShutdownGracePeriod time.Duration
	RequestTimeout      time.Duration

	// Notification delivery (Phase 11).
	//
	// The scheduler runs by default because the notification inbox depends on
	// it: without a pass, nothing is ever materialised and the inbox stays
	// empty. Push is off by default because it needs credentials this
	// repository deliberately does not hold, and an API that refused to start
	// without them would be broken for a feature it is not yet using
	// (plans/phase11.md §§43, 68).
	NotificationSchedulerEnabled  bool
	NotificationSchedulerInterval time.Duration
	NotificationRetention         time.Duration
	PushEnabled                   bool
	// ExpoAccessToken is a secret. It belongs in the environment and never in
	// the repository.
	ExpoAccessToken string
}

// Load reads configuration from the environment, applying defaults and
// validating required values.
func Load() (*Config, error) {
	cfg := &Config{
		Env:                 Environment(getEnvDefault("ENV", string(EnvDevelopment))),
		LogLevel:            getEnvDefault("LOG_LEVEL", "info"),
		CORSAllowedOrigins:  splitAndTrim(os.Getenv("CORS_ALLOWED_ORIGINS")),
		DatabaseURL:         strings.TrimSpace(os.Getenv("DATABASE_URL")),
		SupabaseURL:         strings.TrimSpace(os.Getenv("SUPABASE_URL")),
		SupabaseJWTMode:     JWTMode(getEnvDefault("SUPABASE_JWT_MODE", string(JWTModeAsymmetric))),
		SupabaseJWTSecret:   os.Getenv("SUPABASE_JWT_SECRET"),
		SupabaseJWTAudience: getEnvDefault("SUPABASE_JWT_AUDIENCE", "authenticated"),
		ShutdownGracePeriod: 15 * time.Second,
		RequestTimeout:      30 * time.Second,
		ExpoAccessToken:     strings.TrimSpace(os.Getenv("EXPO_ACCESS_TOKEN")),
	}

	var errs []error

	port, err := getEnvInt("PORT", 8080)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.Port = port

	maxConns, err := getEnvInt("DATABASE_MAX_CONNS", 10)
	if err != nil {
		errs = append(errs, err)
	}
	if maxConns < 1 {
		errs = append(errs, errors.New("DATABASE_MAX_CONNS must be at least 1"))
	}
	cfg.DatabaseMaxConns = int32(maxConns)

	leeway, err := getEnvDuration("SUPABASE_JWT_LEEWAY", 30*time.Second)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.SupabaseJWTLeeway = leeway

	schedulerEnabled, err := getEnvBool("NOTIFICATION_SCHEDULER_ENABLED", true)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.NotificationSchedulerEnabled = schedulerEnabled

	interval, err := getEnvDuration("NOTIFICATION_SCHEDULER_INTERVAL", time.Minute)
	if err != nil {
		errs = append(errs, err)
	}
	if interval < time.Second {
		errs = append(errs, errors.New("NOTIFICATION_SCHEDULER_INTERVAL must be at least 1s"))
	}
	cfg.NotificationSchedulerInterval = interval

	retention, err := getEnvDuration("NOTIFICATION_RETENTION", 30*24*time.Hour)
	if err != nil {
		errs = append(errs, err)
	}
	if retention < 24*time.Hour {
		errs = append(errs, errors.New("NOTIFICATION_RETENTION must be at least 24h"))
	}
	cfg.NotificationRetention = retention

	pushEnabled, err := getEnvBool("PUSH_ENABLED", false)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.PushEnabled = pushEnabled

	switch cfg.Env {
	case EnvDevelopment, EnvTest, EnvStaging, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("ENV must be one of development, test, staging, production (got %q)", cfg.Env))
	}

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	// The project URL is always required: it yields both the JWKS endpoint and
	// the expected token issuer.
	if cfg.SupabaseURL == "" {
		errs = append(errs, errors.New("SUPABASE_URL is required"))
	}
	if cfg.SupabaseJWTAudience == "" {
		errs = append(errs, errors.New("SUPABASE_JWT_AUDIENCE must not be empty"))
	}

	switch cfg.SupabaseJWTMode {
	case JWTModeAsymmetric:
		if strings.TrimSpace(cfg.SupabaseJWTSecret) != "" {
			errs = append(errs, errors.New(
				"SUPABASE_JWT_SECRET must not be set when SUPABASE_JWT_MODE=asymmetric; "+
					"remove the secret or switch to SUPABASE_JWT_MODE=legacy_hs256"))
		}
	case JWTModeLegacyHS256:
		if strings.TrimSpace(cfg.SupabaseJWTSecret) == "" {
			errs = append(errs, errors.New("SUPABASE_JWT_SECRET is required when SUPABASE_JWT_MODE=legacy_hs256"))
		}
	default:
		errs = append(errs, fmt.Errorf(
			"SUPABASE_JWT_MODE must be asymmetric or legacy_hs256 (got %q)", cfg.SupabaseJWTMode))
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return cfg, nil
}

// Addr is the listen address for the HTTP server.
func (c *Config) Addr() string { return fmt.Sprintf(":%d", c.Port) }

// LoadDotEnv loads KEY=VALUE pairs from path into the process environment
// without overwriting variables that are already set. Missing files are
// ignored so production, which injects real environment variables, is
// unaffected.
func LoadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)

		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func getEnvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback, fmt.Errorf("%s must be an integer (got %q)", key, raw)
	}
	return value, nil
}

func getEnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback, fmt.Errorf("%s must be a duration such as 30s (got %q)", key, raw)
	}
	return value, nil
}

func splitAndTrim(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// getEnvBool reads a boolean setting.
//
// Refused rather than guessed when unreadable: PUSH_ENABLED=yes silently
// meaning false is the kind of configuration mistake that is discovered by
// nobody's phone ringing.
func getEnvBool(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback, fmt.Errorf("%s must be true or false (got %q)", key, raw)
	}
	return value, nil
}
