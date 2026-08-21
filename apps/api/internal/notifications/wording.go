package notifications

import (
	"fmt"
	"time"
)

// The words a notification says, and the policy behind them.
//
// A notification appears on a locked phone, in front of whoever happens to be
// holding it — on a table, in a meeting, in someone else's hand. So the wording
// is a privacy decision before it is a copy decision, and the rule is absolute
// rather than a matter of taste (plans/phase11.md §48,
// docs/09-security-privacy.md):
//
//   - Never a medicine's name, its dosage, its form, or its instructions.
//   - Never a condition, a diagnosis, a symptom, or a reason for care.
//   - Never an appointment's title, its clinic, or its practitioner.
//   - Never a task's description.
//
// What is allowed is who it concerns and when, because a caregiver looking
// after two people needs both in order to know whether to act, and neither says
// anything medical. "Amma has an appointment at 10:00" is a sentence anybody
// could overhear; "Amma has a cardiology follow-up at 10:00" is not.
//
// Everything specific is fetched after the app is opened, under the reader's
// own authorization. A test in wording_test.go pins that no composed body
// contains any of the forbidden material, using the domains' own vocabulary.

// titles are the whole of what a glanced-at notification really shows.
var titles = map[Type]string{
	TypeMedicationReminder:  "Medication reminder",
	TypeTaskReminder:        "Care task reminder",
	TypeAppointmentReminder: "Upcoming appointment",
	TypeTaskOverdue:         "Overdue care task",
	TypeCareActivity:        "Care activity",
}

// Title is the notification's first line.
func Title(t Type) string {
	if title, ok := titles[t]; ok {
		return title
	}
	return "MeraCare"
}

// Subject is who a notification is about, and the clock its times are read in.
type Subject struct {
	Name string
	// Timezone is the senior's IANA name. Care happens on the senior's clock,
	// not the reader's: a daughter in London must be told about her mother's
	// 08:00 dose, not about 02:30 (plans/phase11.md §33).
	Timezone string
}

// Body composes the second line of a reminder-shaped notification.
//
// dueAt is when the care is due; readAt is the moment the sentence is written,
// used only to decide whether the day needs naming. Nothing else about the care
// reaches this function, which is the mechanism rather than the intention: it
// cannot name a medicine because it is never given one.
func Body(t Type, subject Subject, dueAt, readAt time.Time) string {
	when := whenPhrase(dueAt, readAt, subject.Timezone)
	name := subject.Name
	if name == "" {
		name = "the person you care for"
	}

	switch t {
	case TypeMedicationReminder:
		return fmt.Sprintf("A dose is due for %s %s.", name, when)
	case TypeTaskReminder:
		return fmt.Sprintf("Something is due for %s %s.", name, when)
	case TypeAppointmentReminder:
		return fmt.Sprintf("%s has an appointment %s.", name, when)
	case TypeTaskOverdue:
		return fmt.Sprintf("Something for %s was due %s and has not been recorded yet.", name, when)
	default:
		return fmt.Sprintf("There is an update in %s's care.", name)
	}
}

// ActivityBody composes the second line of a care-activity notification.
//
// It says who did what kind of thing, and nothing about which thing. "Priya
// recorded a dose" carries no more information to a stranger than "Priya used
// an app", while telling the family exactly what they wanted to know.
func ActivityBody(activity ActivityKind, actorName string, subject Subject) string {
	if actorName == "" {
		actorName = "Someone"
	}
	name := subject.Name
	if name == "" {
		name = "the person you care for"
	}

	switch activity {
	case ActivityMedicationRecorded:
		return fmt.Sprintf("%s recorded a dose for %s.", actorName, name)
	case ActivityTaskCompleted:
		return fmt.Sprintf("%s completed a care task for %s.", actorName, name)
	case ActivityAppointmentCompleted:
		return fmt.Sprintf("%s completed an appointment for %s.", actorName, name)
	case ActivityMemberJoined:
		return fmt.Sprintf("%s joined %s's care circle.", actorName, name)
	default:
		return fmt.Sprintf("There is an update in %s's care.", name)
	}
}

// whenPhrase renders an instant in the senior's own timezone.
//
// The day is named only when it is not the day the sentence is written, so the
// common case reads "at 08:00" rather than "on Tuesday 3 March at 08:00" —
// which is both shorter and, on a lock screen, less of a diary entry.
func whenPhrase(at, readAt time.Time, timezone string) string {
	location := locationFor(timezone)

	local := at.In(location)
	today := readAt.In(location)

	clock := local.Format("15:04")

	switch {
	case sameDay(local, today):
		return "at " + clock
	case sameDay(local, today.AddDate(0, 0, 1)):
		return "tomorrow at " + clock
	case sameDay(local, today.AddDate(0, 0, -1)):
		return "yesterday at " + clock
	default:
		return local.Format("on 2 January") + " at " + clock
	}
}

// locationFor resolves an IANA name, falling back to UTC.
//
// A senior whose timezone is missing or unrecognised gets a time that is
// consistent rather than a notification that fails. cmd/api embeds the tzdata,
// so an unknown name here means the stored value is wrong, not that the zone
// database is absent.
func locationFor(timezone string) *time.Location {
	if timezone == "" {
		return time.UTC
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.UTC
	}
	return location
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
