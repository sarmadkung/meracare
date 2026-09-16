package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/genxcare/api/internal/config"
)

// setValidEnv sets the minimum environment required for Load to succeed.
func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/genxcare")
	t.Setenv("SUPABASE_URL", "https://project.supabase.co")
	t.Setenv("SUPABASE_JWT_MODE", "")
	t.Setenv("SUPABASE_JWT_SECRET", "")
}

func TestLoadDefaults(t *testing.T) {
	setValidEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Env != config.EnvDevelopment {
		t.Errorf("Env = %q, want development", cfg.Env)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.Addr() != ":8080" {
		t.Errorf("Addr() = %q, want :8080", cfg.Addr())
	}
	if cfg.SupabaseJWTAudience != "authenticated" {
		t.Errorf("SupabaseJWTAudience = %q, want authenticated", cfg.SupabaseJWTAudience)
	}
	if cfg.SupabaseJWTLeeway != 30*time.Second {
		t.Errorf("SupabaseJWTLeeway = %v, want 30s", cfg.SupabaseJWTLeeway)
	}
	if cfg.DatabaseMaxConns != 10 {
		t.Errorf("DatabaseMaxConns = %d, want 10", cfg.DatabaseMaxConns)
	}
}

// Asymmetric verification is the default: the API should never need a secret
// capable of minting tokens.
func TestLoadDefaultsToAsymmetricJWTMode(t *testing.T) {
	setValidEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SupabaseJWTMode != config.JWTModeAsymmetric {
		t.Errorf("SupabaseJWTMode = %q, want asymmetric", cfg.SupabaseJWTMode)
	}
}

func TestLoadRequiresDatabaseURLAndSupabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_JWT_SECRET", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() succeeded without DATABASE_URL or SUPABASE_URL")
	}
	for _, want := range []string{"DATABASE_URL", "SUPABASE_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestLoadLegacyModeRequiresSecret(t *testing.T) {
	setValidEnv(t)
	t.Setenv("SUPABASE_JWT_MODE", "legacy_hs256")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() accepted legacy_hs256 without SUPABASE_JWT_SECRET")
	}

	t.Setenv("SUPABASE_JWT_SECRET", "test-secret")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SupabaseJWTMode != config.JWTModeLegacyHS256 {
		t.Errorf("SupabaseJWTMode = %q, want legacy_hs256", cfg.SupabaseJWTMode)
	}
}

// An unused shared secret sitting in the environment is a forgeable key with no
// purpose, so configuring both is rejected rather than silently ignored.
func TestLoadRejectsSecretInAsymmetricMode(t *testing.T) {
	setValidEnv(t)
	t.Setenv("SUPABASE_JWT_SECRET", "left-over-secret")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() accepted a JWT secret in asymmetric mode")
	}
	if !strings.Contains(err.Error(), "SUPABASE_JWT_SECRET") {
		t.Errorf("error %q does not mention SUPABASE_JWT_SECRET", err)
	}
}

func TestLoadRejectsUnknownJWTMode(t *testing.T) {
	setValidEnv(t)
	t.Setenv("SUPABASE_JWT_MODE", "whatever")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() accepted an unknown SUPABASE_JWT_MODE")
	}
}

func TestLoadRejectsUnknownEnvironment(t *testing.T) {
	setValidEnv(t)
	t.Setenv("ENV", "wherever")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() accepted an unknown ENV value")
	}
}

func TestLoadRejectsNonNumericPort(t *testing.T) {
	setValidEnv(t)
	t.Setenv("PORT", "eighty-eighty")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load() accepted a non-numeric PORT")
	}
}

func TestLoadParsesCORSOrigins(t *testing.T) {
	setValidEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://genxcare.app , ,https://admin.genxcare.app ")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := []string{"https://genxcare.app", "https://admin.genxcare.app"}
	if len(cfg.CORSAllowedOrigins) != len(want) {
		t.Fatalf("CORSAllowedOrigins = %v, want %v", cfg.CORSAllowedOrigins, want)
	}
	for i, origin := range want {
		if cfg.CORSAllowedOrigins[i] != origin {
			t.Errorf("CORSAllowedOrigins[%d] = %q, want %q", i, cfg.CORSAllowedOrigins[i], origin)
		}
	}
}

func TestEnvironmentIsDevelopment(t *testing.T) {
	cases := map[config.Environment]bool{
		config.EnvDevelopment: true,
		config.EnvTest:        true,
		config.EnvStaging:     false,
		config.EnvProduction:  false,
	}
	for env, want := range cases {
		if got := env.IsDevelopment(); got != want {
			t.Errorf("%q.IsDevelopment() = %v, want %v", env, got, want)
		}
	}
}

func TestLoadDotEnvDoesNotOverrideExistingValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	contents := "# comment\n\nLOG_LEVEL=debug\nDATABASE_URL=\"postgres://from-file\"\nMALFORMED\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("DATABASE_URL", "postgres://from-environment")
	os.Unsetenv("LOG_LEVEL")
	t.Cleanup(func() { os.Unsetenv("LOG_LEVEL") })

	if err := config.LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}

	if got := os.Getenv("DATABASE_URL"); got != "postgres://from-environment" {
		t.Errorf("DATABASE_URL = %q, want the pre-existing environment value", got)
	}
	if got := os.Getenv("LOG_LEVEL"); got != "debug" {
		t.Errorf("LOG_LEVEL = %q, want debug", got)
	}
}

func TestLoadDotEnvIgnoresMissingFile(t *testing.T) {
	if err := config.LoadDotEnv(filepath.Join(t.TempDir(), "absent.env")); err != nil {
		t.Fatalf("LoadDotEnv on missing file returned %v, want nil", err)
	}
}
