package database_test

import (
	"strings"
	"testing"

	"github.com/genxcare/api/internal/database"
)

func TestLoadMigrationsAreOrderedAndNonEmpty(t *testing.T) {
	migrations, err := database.LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() error = %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("no migrations were embedded")
	}

	var previous int64
	for _, migration := range migrations {
		if migration.Version <= previous {
			t.Errorf("migration %d is out of order (previous %d)", migration.Version, previous)
		}
		previous = migration.Version

		if strings.TrimSpace(migration.Name) == "" {
			t.Errorf("migration %d has no name", migration.Version)
		}
		if strings.TrimSpace(migration.SQL) == "" {
			t.Errorf("migration %d (%s) is empty", migration.Version, migration.Name)
		}
	}
}

// Migrations are forward-only and immutable once applied, so the first one must
// keep its identity.
func TestFirstMigrationIsInit(t *testing.T) {
	migrations, err := database.LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() error = %v", err)
	}

	if migrations[0].Version != 1 || migrations[0].Name != "init" {
		t.Errorf("first migration = %d_%s, want 0001_init", migrations[0].Version, migrations[0].Name)
	}
}

func TestLatestMigrationAddsMissedMedicationAlerts(t *testing.T) {
	migrations, err := database.LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() error = %v", err)
	}

	latest := migrations[len(migrations)-1]
	if latest.Version != 12 || latest.Name != "missed_medication_alerts" {
		t.Errorf("latest migration = %d_%s, want 0012_missed_medication_alerts", latest.Version, latest.Name)
	}
}
