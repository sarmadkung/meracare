// Package paging holds the keyset cursor shared by every paged history in the
// API.
//
// docs/05-api-and-backend-spec.md asks for cursor pagination and specifically
// against OFFSET for large timelines. The position is always the same shape —
// an instant and a tie-breaking id — because every history GenXcare pages is
// ordered by when something was due and then by row, so one implementation
// covers medication doses, appointments, and the activity timeline to come.
//
// It lives here rather than in whichever module needed it first: a second copy
// of an encoder and its decoder is exactly the kind of duplication that drifts
// quietly, and a cursor that two packages disagree about is a page of somebody
// else's care history.
package paging

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrBadCursor is returned when a cursor cannot be read. It is a sentinel so a
// handler can answer 400 without inspecting the message.
var ErrBadCursor = errors.New("paging: unreadable cursor")

// separator divides the two halves of an encoded position. It cannot occur in
// either half: neither RFC 3339 nor a UUID contains a pipe.
const separator = "|"

// EncodeCursor renders a keyset position as one opaque token.
//
// Opaque on purpose: a client that took it apart would come to depend on the
// ordering, and the ordering is ours to change.
func EncodeCursor(at time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString(
		[]byte(at.UTC().Format(time.RFC3339Nano) + separator + id.String()))
}

// DecodeCursor reads a token back.
//
// An empty cursor means the first page and yields two nils, which a query can
// pass straight to a `$1 IS NULL OR ...` predicate. Anything unreadable is
// refused rather than treated as the beginning: silently restarting would show
// somebody the top of a list they were halfway down.
func DecodeCursor(cursor string) (*time.Time, *uuid.UUID, error) {
	if strings.TrimSpace(cursor) == "" {
		return nil, nil, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, nil, ErrBadCursor
	}

	at, id, ok := strings.Cut(string(raw), separator)
	if !ok {
		return nil, nil, ErrBadCursor
	}

	parsedAt, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, nil, ErrBadCursor
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, nil, ErrBadCursor
	}

	return &parsedAt, &parsedID, nil
}
