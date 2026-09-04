package testsupport

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// The integration suite truncates every application table on every call to
// RequireDatabase. Against a development container that is exactly right: each
// test starts from a known state. Against a hosted database it would erase real
// care records — silently, with no confirmation step, and with nothing to
// notice until afterwards.
//
// So the destination is checked before anything connects. This is a structural
// guarantee rather than a rule somebody has to remember, which is the only kind
// worth having when the cost of forgetting is somebody's medication history.
//
// There is deliberately no override. A flag or an environment variable to
// disable this would be set once, in a shell nobody remembers, and the
// protection would be gone. Anybody who genuinely needs to point the suite
// elsewhere can change this file, and explain why in the commit.

// localHosts are the only hosts the integration suite will connect to.
//
// CI counts: GitHub Actions service containers are reached on localhost, so
// restoring the workflow needs no exception here.
var localHosts = []string{"localhost", "127.0.0.1", "::1"}

// RequireLocalHost reports whether a database URL points somewhere it is safe
// to truncate.
//
// It is exported so the check can be reused by any future helper that destroys
// data, and tested on its own.
func RequireLocalHost(rawURL string) error {
	host, err := hostOf(rawURL)
	if err != nil {
		return err
	}

	if host == "" {
		// A connection string with no host at all means a Unix socket or the
		// libpq default, both of which are local by definition.
		return nil
	}

	if slices.Contains(localHosts, strings.ToLower(host)) {
		return nil
	}

	return fmt.Errorf(
		"%s points at %q, which is not a local database; the suite truncates every "+
			"application table and would erase it. Use the container in docker-compose.yml",
		DatabaseURLEnv, host,
	)
}

// hostOf reads the host out of either connection-string form libpq accepts:
// a URL, or space-separated keyword/value pairs.
func hostOf(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)

	if strings.HasPrefix(trimmed, "postgres://") || strings.HasPrefix(trimmed, "postgresql://") {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", fmt.Errorf("%s is not a usable connection string: %w", DatabaseURLEnv, err)
		}

		// Hostname() strips the port and the brackets around an IPv6 literal.
		return parsed.Hostname(), nil
	}

	// Keyword form: `host=localhost port=5432 dbname=genxcare`. Anything the
	// suite cannot read the host out of is refused rather than assumed local.
	for _, field := range strings.Fields(trimmed) {
		key, value, ok := strings.Cut(field, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "host") {
			// Trim quotes and any brackets around an IPv6 literal, so the value
			// compares the same way url.Hostname renders it.
			return strings.Trim(strings.TrimSpace(value), `'"[]`), nil
		}
	}

	if strings.Contains(trimmed, "=") {
		// Keyword form with no host= clause: libpq falls back to a local socket.
		return "", nil
	}

	return "", fmt.Errorf(
		"%s is not a connection string this check can read, so it will not be trusted",
		DatabaseURLEnv,
	)
}
