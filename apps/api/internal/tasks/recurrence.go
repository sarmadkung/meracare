package tasks

import "github.com/genxcare/api/internal/recurrence"

// The repeat rule for a care task is the shared engine in internal/recurrence,
// named here in the vocabulary this package speaks.
//
// These are aliases, not wrappers: a wrapper would need a conversion at every
// boundary, and the two types would drift. Medication schedules use the same
// engine under their own name (plans/phase5.md §3).
type (
	// Recurrence is how often a care task repeats.
	Recurrence = recurrence.Rule
	// Frequency is DAILY or WEEKLY.
	Frequency = recurrence.Frequency
	// TimeOfDay is a wall-clock time in the senior's timezone.
	TimeOfDay = recurrence.TimeOfDay
)

const (
	// FrequencyDaily fires every day.
	FrequencyDaily = recurrence.FrequencyDaily
	// FrequencyWeekly fires on named days of the week.
	FrequencyWeekly = recurrence.FrequencyWeekly
)

var (
	// ErrUnknownFrequency is a repeat setting other than daily or weekly.
	ErrUnknownFrequency = recurrence.ErrUnknownFrequency
	// ErrUnknownWeekday is a day name that is not a day.
	ErrUnknownWeekday = recurrence.ErrUnknownWeekday
	// ErrNoWeekdays is a weekly repeat that names no days.
	ErrNoWeekdays = recurrence.ErrNoWeekdays
)

var (
	// Daily returns the "every day" rule.
	Daily = recurrence.Daily
	// Weekly returns a rule firing on the given days.
	Weekly = recurrence.Weekly
	// ParseRecurrence reads a stored rule.
	ParseRecurrence = recurrence.Parse
	// ParseTimeOfDay reads "HH:MM".
	ParseTimeOfDay = recurrence.ParseTimeOfDay
)
