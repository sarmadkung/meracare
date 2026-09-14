// Package seed fills a development database with a care circle that looks
// lived-in: several people, several roles, and a day that already has a past.
//
// It exists because an empty app cannot be designed against. A screen with one
// task on it never shows what happens at six, a strip with one face never shows
// how it sorts, and nothing at all shows what a missed dose looks like at the
// top of somebody's morning.
package seed

import (
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"
)

// marker opens every identifier the seeder produces.
//
// Rows carry a derived id rather than a random one, and that is what makes the
// seeder safe to point at a database somebody else is also using: a re-run
// deletes exactly what the last run wrote. The alternative — random ids and a
// TRUNCATE to clean up — would mean the tidy-up step is indistinguishable from
// erasing a real care record.
//
// It is legible on purpose. `5eed5eed-…` in a dashboard says who wrote the row
// without anybody having to look it up.
var marker = [4]byte{0x5e, 0xed, 0x5e, 0xed}

// ID derives the identifier for one seeded row.
//
// kind separates the namespaces, so the third task and the third note of the
// same circle do not collide.
func ID(kind, name string) uuid.UUID {
	digest := sha256.Sum256([]byte(kind + ":" + name))

	var id uuid.UUID
	copy(id[:4], marker[:])
	copy(id[4:], digest[:12])

	// Version 4 and the RFC 4122 variant. Neither is true — this is a hash, not
	// a random number — but a uuid that fails to parse as any version is worse
	// than one that reads as the version it most resembles.
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80

	return id
}

// IsSeeded reports whether an identifier was produced by the seeder.
//
// The reset step deletes only rows this returns true for, so nothing a person
// created can be caught by it.
func IsSeeded(id uuid.UUID) bool {
	return id[0] == marker[0] && id[1] == marker[1] && id[2] == marker[2] && id[3] == marker[3]
}

// IDs derives one identifier per index, for the rows a circle repeats.
func IDs(kind, name string, count int) []uuid.UUID {
	ids := make([]uuid.UUID, count)
	for i := range ids {
		ids[i] = ID(kind, fmt.Sprintf("%s:%d", name, i))
	}
	return ids
}
