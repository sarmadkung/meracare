package seed_test

import (
	"strings"
	"testing"

	"github.com/genxcare/api/internal/config"
	"github.com/genxcare/api/internal/seed"
)

/*
The seeder deletes before it writes. Every test here is about making that
survivable: what it is allowed to point at, and what it says when it refuses.
*/

const hosted = "postgresql://u:p@aws-0-ap-northeast-1.pooler.supabase.com:5432/postgres"

func TestAllowsALocalDatabaseWithoutCeremony(t *testing.T) {
	if err := seed.Allow("postgresql://genxcare:genxcare@localhost:55432/genxcare", config.EnvDevelopment, false); err != nil {
		t.Fatalf("a local database should need no confirmation: %v", err)
	}
}

// The default has to be the safe one. Somebody reaching for the seeder at the
// end of a long day should have to say the dangerous thing out loud.
func TestRefusesAHostedDatabaseUntilItIsConfirmed(t *testing.T) {
	err := seed.Allow(hosted, config.EnvDevelopment, false)
	if err == nil {
		t.Fatal("a hosted database should not be seeded without confirmation")
	}
	if !strings.Contains(err.Error(), "pooler.supabase.com") {
		t.Errorf("the refusal should name the host it refused: %v", err)
	}
}

func TestSeedsAHostedDatabaseOnceConfirmed(t *testing.T) {
	if err := seed.Allow(hosted, config.EnvDevelopment, true); err != nil {
		t.Fatalf("confirmed hosted seeding should be allowed: %v", err)
	}
}

// No flag, no confirmation, no argument. Production is not a place this runs.
func TestRefusesProductionEvenWhenConfirmed(t *testing.T) {
	if err := seed.Allow(hosted, config.EnvProduction, true); err == nil {
		t.Fatal("production should be refused whatever the caller confirms")
	}
	if err := seed.Allow("postgresql://localhost/genxcare", config.EnvProduction, true); err == nil {
		t.Fatal("production should be refused even on a local host")
	}
}
