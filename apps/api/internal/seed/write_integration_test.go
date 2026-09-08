package seed_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/meracare/api/internal/database"
	"github.com/meracare/api/internal/seed"
	"github.com/meracare/api/internal/testsupport"
)

/*
The writer, against a real database.

Two things are worth proving here and cannot be proved anywhere else: that every
row satisfies the constraints the schema imposes, and that re-seeding replaces
the previous run without touching anything else in the table.
*/

func TestWritesAWholeWorldTheSchemaAccepts(t *testing.T) {
	pool := testsupport.RequireDatabase(t)
	ctx := context.Background()

	if err := writeSeed(ctx, t, pool); err != nil {
		t.Fatalf("first run: %v", err)
	}

	seniors := count(ctx, t, pool, `SELECT count(*) FROM senior_profiles`)
	if seniors == 0 {
		t.Fatal("nothing was written")
	}

	// Every screen the seeder exists to fill needs its own rows to exist.
	for query, what := range map[string]string{
		`SELECT count(*) FROM medication_instances WHERE status = 'taken'`:   "doses already taken",
		`SELECT count(*) FROM care_task_instances WHERE status = 'pending'`:  "work still to do",
		`SELECT count(*) FROM appointments WHERE status = 'cancelled'`:       "a cancelled appointment",
		`SELECT count(*) FROM care_events`:                                   "an activity feed",
		`SELECT count(*) FROM notifications WHERE read_at IS NULL`:           "an unread inbox",
		`SELECT count(*) FROM messages`:                                      "a conversation",
		`SELECT count(*) FROM invitations WHERE status = 'pending'`:          "an invitation waiting",
		`SELECT count(*) FROM care_relationships WHERE status = 'revoked'`:   "somebody who left",
		`SELECT count(*) FROM senior_profiles WHERE archived_at IS NOT NULL`: "care that has ended",
	} {
		if count(ctx, t, pool, query) == 0 {
			t.Errorf("nothing seeded %s", what)
		}
	}

	// Re-running is how this tool is used: change the plan, run it again. It has
	// to replace the last run rather than pile a second one on top.
	if err := writeSeed(ctx, t, pool); err != nil {
		t.Fatalf("second run: %v", err)
	}

	if after := count(ctx, t, pool, `SELECT count(*) FROM senior_profiles`); after != seniors {
		t.Errorf("re-seeding left %d seniors, want %d — the previous run was not replaced", after, seniors)
	}
}

/*
The reason the identifiers are derived rather than random.

A development database is rarely only the seeder's. Anything a person created by
using the app has to survive a re-seed, because the alternative — truncating to
clean up — is the same operation as erasing a real care record.
*/
func TestLeavesRowsItDidNotWriteAlone(t *testing.T) {
	pool := testsupport.RequireDatabase(t)
	ctx := context.Background()

	if err := writeSeed(ctx, t, pool); err != nil {
		t.Fatalf("first run: %v", err)
	}

	var author uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM users LIMIT 1`).Scan(&author); err != nil {
		t.Fatal(err)
	}

	// A profile somebody made by using the app, rather than by seeding.
	theirs := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO senior_profiles (id, created_by_user_id, display_name, timezone)
		VALUES ($1, $2, 'A profile somebody made', 'Asia/Karachi')`,
		theirs, author,
	); err != nil {
		t.Fatal(err)
	}

	if err := writeSeed(ctx, t, pool); err != nil {
		t.Fatalf("second run: %v", err)
	}

	var survived bool
	if err := pool.QueryRow(ctx,
		`SELECT exists(SELECT 1 FROM senior_profiles WHERE id = $1)`, theirs,
	).Scan(&survived); err != nil {
		t.Fatal(err)
	}
	if !survived {
		t.Error("re-seeding deleted a profile the seeder did not write")
	}
}

// writeSeed runs one whole seed, with derived identities standing in for the
// Supabase accounts the command would create.
func writeSeed(ctx context.Context, t *testing.T, pool *database.Pool) error {
	t.Helper()

	data, err := seed.Build(seed.NewPlan("example.com"), time.Now())
	if err != nil {
		return err
	}

	for i, row := range data.Users {
		if row.AuthUserID == uuid.Nil {
			data.Users[i].AuthUserID = seed.ID("test-auth", row.PersonaKey)
		}
	}

	users, err := seed.EnsureUsers(ctx, pool, data.Users)
	if err != nil {
		return err
	}

	return seed.Write(ctx, pool, data, users)
}

func count(ctx context.Context, t *testing.T, pool *database.Pool, query string) int {
	t.Helper()

	var n int
	if err := pool.QueryRow(ctx, query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}
