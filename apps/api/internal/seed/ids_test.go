package seed_test

import (
	"testing"

	"github.com/genxcare/api/internal/seed"
)

/*
Seeded rows carry derived identifiers rather than random ones.

That is what makes the seeder safe to point at a shared database: re-running it
deletes exactly the rows it wrote last time and nothing a person created. A
random uuid would leave the previous run behind on every run, and the only way
to clean up would be to truncate the table — which on a hosted database means
deleting somebody's real care record.
*/

func TestTheSameNameAlwaysYieldsTheSameID(t *testing.T) {
	if seed.ID("senior", "amina") != seed.ID("senior", "amina") {
		t.Fatal("a seeded id must survive the process that made it")
	}
}

func TestDifferentThingsGetDifferentIDs(t *testing.T) {
	if seed.ID("senior", "amina") == seed.ID("senior", "yusuf") {
		t.Error("two seniors share an id")
	}
	// The kind is part of the name, so a task and a note numbered alike do not
	// collide.
	if seed.ID("task", "amina:1") == seed.ID("note", "amina:1") {
		t.Error("two kinds of row share an id")
	}
}

// Nothing outside the seeder should ever produce one of these by accident.
func TestSeededIDsAreRecognisable(t *testing.T) {
	if !seed.IsSeeded(seed.ID("senior", "amina")) {
		t.Error("a seeded id is not recognised as one")
	}
}
