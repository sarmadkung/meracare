// Package testsupport provides shared helpers for integration tests that need
// a real PostgreSQL database.
//
// Integration tests are skipped unless TEST_DATABASE_URL is set, so
// `go test ./...` stays fast and hermetic while CI and local developers can run
// the full suite against `docker compose up -d postgres`.
package testsupport

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/genxcare/api/internal/database"
	"github.com/genxcare/api/pkg/logging"
)

// DatabaseURLEnv names the environment variable that enables integration tests.
const DatabaseURLEnv = "TEST_DATABASE_URL"

// RequireDatabase returns a migrated connection pool, skipping the test when no
// test database is configured.
//
// Each call truncates application tables so tests start from a known state.
func RequireDatabase(t *testing.T) *database.Pool {
	t.Helper()

	loadDotEnv(t)

	url := os.Getenv(DatabaseURLEnv)
	if url == "" {
		t.Skipf("set %s to run integration tests (see docker-compose.yml)", DatabaseURLEnv)
	}
	if err := RequireLocalHost(url); err != nil {
		t.Fatalf("refusing to run integration tests: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, database.Options{URL: url, MaxConns: 4})
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := database.Migrate(ctx, pool, logging.New(io.Discard, logging.Options{Level: "error"})); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	lock(ctx, t, pool)
	truncate(ctx, t, pool)

	return pool
}

// exclusiveLockKey is an arbitrary constant identifying the test database lock.
const exclusiveLockKey = 4726_2026

// lock gives one test exclusive use of the database until it finishes.
//
// `go test ./...` runs packages in parallel, and every package that uses the
// database truncates the same tables. Without this, one package's TRUNCATE
// deletes the users another package is midway through building on, which
// surfaces as a foreign-key violation in a test that has nothing wrong with it.
//
// A PostgreSQL advisory lock serialises them with no flag for anybody to
// remember: `go test ./...` is correct on its own, which `-p 1` would not be.
// The lock is session-scoped, so it must be taken and released on one pinned
// connection rather than anywhere in the pool.
func lock(ctx context.Context, t *testing.T, pool *database.Pool) {
	t.Helper()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire test database lock connection: %v", err)
	}

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", exclusiveLockKey); err != nil {
		conn.Release()
		t.Fatalf("lock test database: %v", err)
	}

	t.Cleanup(func() {
		// Best effort: the connection is released either way, and a session
		// that ends drops its advisory locks regardless.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, _ = conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", exclusiveLockKey)
		conn.Release()
	})
}

// applicationTables is every table holding application data, listed explicitly
// so a new table added in a later phase is a deliberate decision here rather
// than silently leaking rows between tests.
var applicationTables = []string{
	"message_read_states",
	"messages",
	"care_notes",
	"notification_devices",
	"notification_preferences",
	"care_events",
	"appointments",
	"medication_instances",
	"medication_schedules",
	"medications",
	"care_task_instances",
	"care_task_templates",
	"invitations",
	"care_relationships",
	"senior_profiles",
	"users",
}

// truncate empties every application table. schema_migrations is preserved so
// the schema is migrated once per database rather than once per test.
func truncate(ctx context.Context, t *testing.T, pool *database.Pool) {
	t.Helper()

	statement := "TRUNCATE TABLE " + strings.Join(applicationTables, ", ") + " RESTART IDENTITY CASCADE"
	if _, err := pool.Exec(ctx, statement); err != nil {
		t.Fatalf("truncate test database: %v", err)
	}
}
