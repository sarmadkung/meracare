package careevents_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/genxcare/api/internal/careevents"
)

// The vocabulary is the documentation's, so the tests that matter here are the
// ones that stop it drifting: from the documentation, from the database, and
// from what the code actually emits.

func TestTheVocabularyMatchesTheDocumentedNames(t *testing.T) {
	// docs/03-domain-model.md names these; plans/phase7.md §2 adds the rest.
	want := []careevents.Type{
		"MEMBER_INVITED", "MEMBER_JOINED", "MEMBER_REVOKED",
		"TASK_CREATED", "TASK_COMPLETED", "TASK_SKIPPED", "TASK_MISSED",
		"MEDICATION_CREATED", "MEDICATION_TAKEN", "MEDICATION_SKIPPED", "MEDICATION_MISSED",
		"APPOINTMENT_CREATED", "APPOINTMENT_COMPLETED", "APPOINTMENT_CANCELLED",
		"NOTE_ADDED",
	}

	if len(careevents.Types) != len(want) {
		t.Fatalf("vocabulary has %d types, want %d:\n got %v\nwant %v",
			len(careevents.Types), len(want), careevents.Types, want)
	}
	for _, wanted := range want {
		if !wanted.Valid() {
			t.Errorf("%q is documented but not recognised", wanted)
		}
	}
}

// A name outside the vocabulary must be refused rather than stored, which is
// what stops a caller inventing a parallel naming system (plans/phase7.md §2).
func TestInventedEventNamesAreNotRecognised(t *testing.T) {
	for _, invented := range []careevents.Type{
		"TASK_UPDATED", "MEDICATION_EDITED", "task_completed", "", "SOMETHING_HAPPENED",
	} {
		if invented.Valid() {
			t.Errorf("%q is recognised, but the documentation names no such event", invented)
		}
	}
}

// The CHECK constraint is the last line of defence and has to list exactly the
// same names. Reading it from the migration keeps the two from drifting apart
// silently — a mismatch here is a runtime failure nobody would see until an
// event failed to write in production.
func TestTheDatabaseConstraintListsTheSameNames(t *testing.T) {
	migration, err := os.ReadFile(filepath.Join(
		"..", "database", "migrations", "0007_care_events.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	block := regexp.MustCompile(`(?s)care_events_type_recognised\s*CHECK \(event_type IN \((.*?)\)\)`)
	found := block.FindSubmatch(migration)
	if found == nil {
		t.Fatal("could not find the event_type CHECK constraint in the migration")
	}

	inConstraint := map[string]bool{}
	for _, quoted := range regexp.MustCompile(`'([A-Z_]+)'`).FindAllStringSubmatch(string(found[1]), -1) {
		inConstraint[quoted[1]] = true
	}

	for _, eventType := range careevents.Types {
		if !inConstraint[string(eventType)] {
			t.Errorf("%q is in the Go vocabulary but not in the database CHECK", eventType)
		}
		delete(inConstraint, string(eventType))
	}
	for leftover := range inConstraint {
		t.Errorf("%q is in the database CHECK but not in the Go vocabulary", leftover)
	}
}

func TestEntityTypesAreRecognised(t *testing.T) {
	for _, entity := range careevents.EntityTypes {
		if !entity.Valid() {
			t.Errorf("%q is listed but not recognised", entity)
		}
	}
	for _, invented := range []careevents.EntityType{"senior", "user", ""} {
		if invented.Valid() {
			t.Errorf("%q is recognised, but no event points at one", invented)
		}
	}
}

// The documented-but-unemitted types are a deliberate decision, not an
// oversight, and this pins the decision: nothing in the codebase produces one.
//
// TASK_MISSED and MEDICATION_MISSED are derived from the clock rather than
// performed by anybody — nothing writes "missed" anywhere, precisely so no
// background sweep has to be alive for the data to be true. Emitting them would
// mean inventing the sweep Phases 4 and 5 refused. NOTE_ADDED has no domain yet.
func TestDocumentedButUnemittedTypesAreNotEmittedAnywhere(t *testing.T) {
	sources := goSources(t, filepath.Join("..", ".."))

	for _, unemitted := range careevents.NotYetEmitted {
		// The declaration and this test are the only places the name may appear.
		allowed := map[string]bool{
			filepath.Join("internal", "careevents", "event.go"):      true,
			filepath.Join("internal", "careevents", "event_test.go"): true,
		}

		for path, content := range sources {
			if allowed[path] || !strings.Contains(content, string(unemitted)) {
				continue
			}
			t.Errorf("%s mentions %q, which is documented but deliberately never emitted",
				path, unemitted)
		}
	}
}

// goSources reads every .go file under root, keyed by its path relative to root.
func goSources(t *testing.T, root string) map[string]string {
	t.Helper()

	sources := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sources[relative] = string(content)
		return nil
	})
	if err != nil {
		t.Fatalf("walk sources: %v", err)
	}
	return sources
}
