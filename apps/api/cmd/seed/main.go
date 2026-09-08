// Command seed fills a development database with a care circle that looks
// lived-in.
//
//	go run ./cmd/seed                       seed the database in DATABASE_URL
//	go run ./cmd/seed -dry-run              print what would be written
//	go run ./cmd/seed -confirm-not-local    required when the target is hosted
//
// It replaces its own previous run and nothing else: every row it writes
// carries a derived identifier, and the clean-up step deletes only those.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/meracare/api/internal/config"
	"github.com/meracare/api/internal/database"
	"github.com/meracare/api/internal/seed"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "meracare-seed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		domain   = flag.String("domain", "example.com", "email domain for the seeded accounts")
		password = flag.String("password", "MeraCare-Seed-2026!", "password every seeded account shares")
		anonKey  = flag.String("anon-key", "", "Supabase anon key (defaults to the mobile app's)")
		confirm  = flag.Bool("confirm-not-local", false, "seed a database that is not local")
		dryRun   = flag.Bool("dry-run", false, "print what would be written and stop")
	)
	flag.Parse()

	// The API's own environment, then the mobile app's, which is where the anon
	// key lives — it is a publishable key that ships inside the bundle, so
	// reading it here adds no exposure.
	if err := config.LoadDotEnv(".env"); err != nil {
		return fmt.Errorf("load .env: %w", err)
	}
	if err := config.LoadDotEnv("../mobile/.env"); err != nil {
		return fmt.Errorf("load ../mobile/.env: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	permitted := seed.Allow(cfg.DatabaseURL, cfg.Env, *confirm)

	plan := seed.NewPlan(*domain)
	data, err := seed.Build(plan, time.Now())
	if err != nil {
		return err
	}

	// A dry run writes nothing, so it is allowed to describe a destination it
	// would refuse — that is the most useful moment to be told why.
	if *dryRun {
		describe(plan, data, cfg.DatabaseURL, permitted)
		return nil
	}

	if permitted != nil {
		return permitted
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	key := *anonKey
	if key == "" {
		key = firstNonEmpty(os.Getenv("SUPABASE_ANON_KEY"), os.Getenv("EXPO_PUBLIC_SUPABASE_ANON_KEY"))
	}
	if key == "" {
		return fmt.Errorf(
			"no Supabase anon key. Pass -anon-key, or set SUPABASE_ANON_KEY, or put " +
				"EXPO_PUBLIC_SUPABASE_ANON_KEY in apps/mobile/.env",
		)
	}

	accounts := seed.Accounts{URL: cfg.SupabaseURL, AnonKey: key, Password: *password}

	fmt.Printf("Creating sign-in accounts at %s\n", cfg.SupabaseURL)
	for i, row := range data.Users {
		if row.AuthUserID != uuid.Nil {
			continue // Somebody who never signs in; their identity is derived.
		}

		id, err := accounts.Ensure(ctx, row.Email)
		if err != nil {
			return err
		}
		data.Users[i].AuthUserID = id
		fmt.Printf("  %-14s %s\n", row.PersonaKey, row.Email)
	}

	pool, err := database.Connect(ctx, database.Options{URL: cfg.DatabaseURL, MaxConns: cfg.DatabaseMaxConns})
	if err != nil {
		return err
	}
	defer pool.Close()

	users, err := seed.EnsureUsers(ctx, pool, data.Users)
	if err != nil {
		return err
	}

	if err := seed.Write(ctx, pool, data, users); err != nil {
		return err
	}

	report(plan, data, *password)
	return nil
}

func describe(plan seed.Plan, data seed.Dataset, databaseURL string, permitted error) {
	fmt.Printf("Would write to %s\n", redact(databaseURL))
	if permitted != nil {
		fmt.Printf("  ...except that it would not: %v\n", permitted)
	}
	fmt.Println()
	fmt.Printf("  %d people, %d circles\n", len(data.Users), len(data.Seniors))
	fmt.Printf("  %d medicines on %d schedules, %d doses\n",
		len(data.Medications), len(data.Schedules), len(data.Doses))
	fmt.Printf("  %d tasks, %d appointments\n", len(data.Tasks), len(data.Appointments))
	fmt.Printf("  %d notes, %d messages, %d activity entries, %d notifications\n\n",
		len(data.Notes), len(data.Messages), len(data.Events), len(data.Notifications))

	for _, persona := range plan.Personas {
		if !persona.SignsIn {
			continue
		}
		fmt.Printf("  %-24s %s\n", persona.Email, summarise(plan, persona.Key))
	}
}

func report(plan seed.Plan, data seed.Dataset, password string) {
	fmt.Printf("\nSeeded %d circles. Sign in with the password %q.\n\n", len(data.Seniors), password)

	for _, persona := range plan.Personas {
		if !persona.SignsIn {
			continue
		}
		fmt.Printf("  %-24s %-14s %s\n", persona.Email, persona.Name, summarise(plan, persona.Key))
	}

	for _, circle := range plan.Circles {
		for _, invitation := range circle.Invitations {
			fmt.Printf("\n  Invitation waiting for %s in %s:\n    token %s\n",
				invitation.Email, circle.Name, seed.InviteToken(circle.Key, invitation.Email))
		}
	}

	fmt.Println()
}

// summarise says what a persona will actually see, which is the only reason to
// pick one of them over another.
func summarise(plan seed.Plan, personaKey string) string {
	circles := plan.CirclesFor(personaKey)

	names := make([]string, 0, len(circles))
	zones := map[string]bool{}
	for _, circle := range circles {
		names = append(names, circle.Name)
		zones[circle.Timezone] = true
	}

	suffix := ""
	if len(zones) > 1 {
		suffix = fmt.Sprintf(" (%d timezones)", len(zones))
	}

	noun := "circles"
	if len(circles) == 1 {
		noun = "circle"
	}

	return fmt.Sprintf("%d %s: %s%s", len(circles), noun, strings.Join(names, ", "), suffix)
}

// redact keeps a password out of a printed connection string.
func redact(databaseURL string) string {
	at := strings.LastIndex(databaseURL, "@")
	scheme := strings.Index(databaseURL, "://")
	if at < 0 || scheme < 0 || at < scheme {
		return databaseURL
	}
	return databaseURL[:scheme+3] + "…" + databaseURL[at:]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
