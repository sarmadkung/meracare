package paging_test

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/paging"
)

func TestACursorSurvivesTheRoundTrip(t *testing.T) {
	at := time.Date(2026, time.August, 17, 9, 30, 0, 123456789, time.UTC)
	id := uuid.New()

	decodedAt, decodedID, err := paging.DecodeCursor(paging.EncodeCursor(at, id))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decodedAt == nil || !decodedAt.Equal(at) {
		t.Errorf("instant = %v, want %v", decodedAt, at)
	}
	if decodedID == nil || *decodedID != id {
		t.Errorf("id = %v, want %v", decodedID, id)
	}
}

// A cursor made in one zone must name the same instant when read in another:
// the position is a moment, not a wall clock.
func TestACursorIsAnInstantNotALocalTime(t *testing.T) {
	karachi, err := time.LoadLocation("Asia/Karachi")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}

	at := time.Date(2026, time.August, 17, 9, 30, 0, 0, karachi)
	id := uuid.New()

	decodedAt, _, err := paging.DecodeCursor(paging.EncodeCursor(at, id))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !decodedAt.Equal(at) {
		t.Errorf("instant = %v, want the same moment as %v", decodedAt, at)
	}
}

func TestAnEmptyCursorMeansTheFirstPage(t *testing.T) {
	for _, cursor := range []string{"", "   "} {
		at, id, err := paging.DecodeCursor(cursor)
		if err != nil {
			t.Fatalf("decode %q: %v", cursor, err)
		}
		if at != nil || id != nil {
			t.Errorf("decode %q = (%v, %v), want the start of the list", cursor, at, id)
		}
	}
}

// An unreadable cursor is refused rather than quietly restarting the list.
func TestAnUnreadableCursorIsRefused(t *testing.T) {
	cases := map[string]string{
		"not base64":        "not a cursor!",
		"no separator":      base64.RawURLEncoding.EncodeToString([]byte("2026-08-17T09:30:00Z")),
		"unparseable time":  base64.RawURLEncoding.EncodeToString([]byte("yesterday|" + uuid.New().String())),
		"unparseable id":    base64.RawURLEncoding.EncodeToString([]byte("2026-08-17T09:30:00Z|nope")),
		"empty after split": base64.RawURLEncoding.EncodeToString([]byte("|")),
	}

	for name, cursor := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := paging.DecodeCursor(cursor); !errors.Is(err, paging.ErrBadCursor) {
				t.Errorf("err = %v, want ErrBadCursor", err)
			}
		})
	}
}
