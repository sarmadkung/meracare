package testsupport

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/genxcare/api/internal/config"
)

// Tests read apps/api/.env, the same file the API itself reads.
//
// Without this, TEST_DATABASE_URL had to be exported by hand on every
// invocation — so `go test ./...` silently skipped every integration test, and
// the way to find that out was to notice the word SKIP scroll past. Reading the
// file somebody has already filled in makes the default behaviour the useful
// one.
//
// config.LoadDotEnv never overwrites a variable that is already set, so an
// inline `TEST_DATABASE_URL=... go test` and CI's own environment both still
// win over the file.

var dotEnvOnce sync.Once

// loadDotEnv loads apps/api/.env into the environment, once per test binary.
func loadDotEnv(t *testing.T) {
	t.Helper()

	dotEnvOnce.Do(func() {
		root, err := moduleRoot()
		if err != nil {
			// Not fatal: the environment may well be set directly, which is how
			// CI runs. The caller's own check reports a missing URL.
			return
		}
		if err := config.LoadDotEnv(filepath.Join(root, ".env")); err != nil {
			t.Fatalf("read %s: %v", filepath.Join(root, ".env"), err)
		}
	})
}

// moduleRoot walks up from the test's working directory to the directory
// holding go.mod, which is apps/api.
//
// Tests run in the directory of the package under test, so the path to .env is
// not fixed relative to the caller — it has to be found.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
