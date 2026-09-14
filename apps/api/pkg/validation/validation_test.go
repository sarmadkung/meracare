package validation_test

import (
	"testing"

	"github.com/genxcare/api/pkg/validation"
)

func TestErrorsAddKeepsFirstMessage(t *testing.T) {
	var errs validation.Errors
	if errs.Any() {
		t.Fatal("a zero-value Errors should be empty")
	}

	errs.Add("displayName", "first")
	errs.Add("displayName", "second")

	if !errs.Any() {
		t.Fatal("Any() = false after Add")
	}
	if errs["displayName"] != "first" {
		t.Errorf("displayName = %q, want first", errs["displayName"])
	}
}

func TestRequired(t *testing.T) {
	var errs validation.Errors
	errs.Required("displayName", "   ")
	errs.Required("phone", "+92 300 1234567")

	if _, failed := errs["displayName"]; !failed {
		t.Error("blank value should fail Required")
	}
	if _, failed := errs["phone"]; failed {
		t.Error("non-blank value should pass Required")
	}
}

func TestMaxLength(t *testing.T) {
	var errs validation.Errors
	errs.MaxLength("displayName", "Sara", 4)
	errs.MaxLength("notes", "Salamü", 5)

	if _, failed := errs["displayName"]; failed {
		t.Error("value at the limit should pass MaxLength")
	}
	if _, failed := errs["notes"]; !failed {
		t.Error("value over the limit should fail MaxLength")
	}
}

func TestTrimmedLengthCountsRunesNotBytes(t *testing.T) {
	if got := validation.TrimmedLength("  Salamü  "); got != 6 {
		t.Errorf("TrimmedLength = %d, want 6", got)
	}
}
