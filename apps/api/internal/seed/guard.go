package seed

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/meracare/api/internal/config"
)

// localHosts are the databases a seed run may write to without being asked
// twice. CI counts: service containers are reached on localhost.
var localHosts = []string{"localhost", "127.0.0.1", "::1"}

// ConfirmFlag is the flag a caller must pass to seed a database that is not
// local. Named here so the refusal below and the command that prints it cannot
// drift apart.
const ConfirmFlag = "-confirm-not-local"

// Allow reports whether a seed run may proceed against this database.
//
// The seeder deletes its own previous rows before writing new ones, so the
// destination is checked before anything connects. Two rules, in order of how
// badly they end:
//
//   - Production is refused outright. There is no flag, and adding one would
//     only mean somebody eventually passes it.
//   - Anything that is not a local database is refused until the caller says so
//     explicitly. Development against hosted Supabase is a normal way to work,
//     but it should never be the thing that happens by default.
//
// This is deliberately narrower than internal/testsupport.RequireLocalHost,
// which has no confirmation path at all: the integration suite truncates every
// table, while the seeder only ever deletes rows carrying its own marker.
func Allow(databaseURL string, env config.Environment, confirmed bool) error {
	if env == config.EnvProduction {
		return fmt.Errorf("seed: ENV is %q; the seeder does not run against production", env)
	}

	host := hostOf(databaseURL)
	if host == "" || slices.Contains(localHosts, strings.ToLower(host)) {
		return nil
	}

	if !confirmed {
		return fmt.Errorf(
			"seed: DATABASE_URL points at %q, which is not a local database. "+
				"Seeding deletes the rows a previous run wrote there. "+
				"Pass %s if that is what you mean, or run the container in docker-compose.yml",
			host, ConfirmFlag,
		)
	}

	return nil
}

// hostOf reads the host out of a connection string.
//
// An unparseable string, or one with no host, yields "": both mean a Unix
// socket or the libpq default, which are local by definition. Anything the
// parser cannot make sense of falls through to the confirmation rule rather
// than being trusted, because "" here would be a guess.
func hostOf(databaseURL string) string {
	trimmed := strings.TrimSpace(databaseURL)
	if trimmed == "" {
		return ""
	}

	if parsed, err := url.Parse(trimmed); err == nil && parsed.Host != "" {
		return parsed.Hostname()
	}

	// The keyword/value form libpq also accepts: "host=db port=5432 ...".
	for _, field := range strings.Fields(trimmed) {
		if after, ok := strings.CutPrefix(field, "host="); ok {
			return after
		}
	}

	return ""
}
