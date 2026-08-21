package notifications

import (
	"strings"
	"testing"
	"time"
)

// The lock-screen privacy policy, pinned.
//
// These are the tests that would fail if somebody helpfully added the medicine's
// name to a reminder. That is worth failing for: a notification appears in front
// of whoever is near the phone, and "Take your 500mg Metformin" tells a room
// full of people that somebody has diabetes (plans/phase11.md §48,
// docs/09-security-privacy.md).

// forbidden is the kind of material no notification may ever contain. Written
// as the things a real MeraCare record holds, because those are what would
// actually leak if the policy were relaxed.
var forbidden = []string{
	// Medicines, dosages, forms.
	"Metformin", "Amlodipine", "Insulin", "500 mg", "500mg", "tablet", "capsule",
	// Conditions and reasons for care.
	"diabetes", "heart", "cardiology", "dementia", "blood pressure",
	// Appointment and task specifics.
	"Dr ", "clinic", "physiotherapy", "blood test",
}

func TestNoNotificationBodyContainsAnythingMedical(t *testing.T) {
	t.Parallel()

	// Every type, with a subject whose own name is innocuous, so anything
	// medical in the output could only have come from the wording itself.
	subject := Subject{Name: "Amma", Timezone: "Asia/Kolkata"}
	dueAt := time.Date(2026, 3, 1, 8, 0, 0, 0, kolkata)
	readAt := dueAt.Add(-15 * time.Minute)

	sentences := make([]string, 0)
	for _, notificationType := range Types {
		sentences = append(sentences, Title(notificationType))
		sentences = append(sentences, Body(notificationType, subject, dueAt, readAt))
	}
	for _, kind := range []ActivityKind{
		ActivityMedicationRecorded,
		ActivityTaskCompleted,
		ActivityAppointmentCompleted,
		ActivityMemberJoined,
	} {
		sentences = append(sentences, ActivityBody(kind, "Priya", subject))
	}

	for _, sentence := range sentences {
		lowered := strings.ToLower(sentence)
		for _, banned := range forbidden {
			if strings.Contains(lowered, strings.ToLower(banned)) {
				t.Errorf("notification text %q contains %q, which must never reach a lock screen",
					sentence, banned)
			}
		}
	}
}

func TestEveryTypeHasWordingOfItsOwn(t *testing.T) {
	t.Parallel()

	subject := Subject{Name: "Amma", Timezone: "Asia/Kolkata"}
	dueAt := time.Date(2026, 3, 1, 8, 0, 0, 0, kolkata)

	seen := make(map[string]Type, len(Types))
	for _, notificationType := range Types {
		title := Title(notificationType)
		if title == "" || title == "MeraCare" {
			t.Errorf("%s has no title of its own", notificationType)
		}
		if previous, ok := seen[title]; ok {
			t.Errorf("%s and %s share the title %q", previous, notificationType, title)
		}
		seen[title] = notificationType

		if body := Body(notificationType, subject, dueAt, dueAt); body == "" {
			t.Errorf("%s has no body", notificationType)
		}
	}
}

func TestTheBodySaysWhoAndWhenAndNothingElse(t *testing.T) {
	t.Parallel()

	subject := Subject{Name: "Amma", Timezone: "Asia/Kolkata"}
	dueAt := time.Date(2026, 3, 1, 8, 0, 0, 0, kolkata)

	body := Body(TypeMedicationReminder, subject, dueAt, dueAt.Add(-15*time.Minute))

	if !strings.Contains(body, "Amma") {
		t.Errorf("body %q does not say who it is about", body)
	}
	if !strings.Contains(body, "08:00") {
		t.Errorf("body %q does not say when", body)
	}
}

func TestTheTimeIsReadInTheSeniorsClockNotTheServersOne(t *testing.T) {
	t.Parallel()

	// The same instant, described to a reader whose senior lives in Kolkata and
	// to one whose senior lives in London. Both must read the senior's clock.
	instant := time.Date(2026, 3, 1, 2, 30, 0, 0, time.UTC)

	indian := Body(TypeMedicationReminder,
		Subject{Name: "Amma", Timezone: "Asia/Kolkata"}, instant, instant)
	british := Body(TypeMedicationReminder,
		Subject{Name: "Dad", Timezone: "Europe/London"}, instant, instant)

	if !strings.Contains(indian, "08:00") {
		t.Errorf("Kolkata body %q should read 08:00", indian)
	}
	if !strings.Contains(british, "02:30") {
		t.Errorf("London body %q should read 02:30", british)
	}
}

func TestAnUnknownTimezoneStillProducesASentence(t *testing.T) {
	t.Parallel()

	// A missing or wrong zone must not stop a reminder. It falls back to UTC,
	// which is consistent and visibly wrong, rather than failing silently.
	instant := time.Date(2026, 3, 1, 2, 30, 0, 0, time.UTC)

	for _, timezone := range []string{"", "Mars/Olympus_Mons"} {
		body := Body(TypeMedicationReminder, Subject{Name: "Amma", Timezone: timezone}, instant, instant)
		if !strings.Contains(body, "02:30") {
			t.Errorf("timezone %q produced %q, want the UTC fallback", timezone, body)
		}
	}
}

func TestTheDayIsNamedOnlyWhenItIsNotToday(t *testing.T) {
	t.Parallel()

	subject := Subject{Name: "Amma", Timezone: "Asia/Kolkata"}
	readAt := time.Date(2026, 3, 1, 9, 0, 0, 0, kolkata)

	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"today", time.Date(2026, 3, 1, 18, 0, 0, 0, kolkata), "at 18:00"},
		{"tomorrow", time.Date(2026, 3, 2, 10, 0, 0, 0, kolkata), "tomorrow at 10:00"},
		{"yesterday", time.Date(2026, 2, 28, 10, 0, 0, 0, kolkata), "yesterday at 10:00"},
		{"further out", time.Date(2026, 3, 9, 10, 0, 0, 0, kolkata), "on 9 March at 10:00"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			body := Body(TypeAppointmentReminder, subject, testCase.at, readAt)
			if !strings.Contains(body, testCase.want) {
				t.Errorf("body = %q, want it to contain %q", body, testCase.want)
			}
		})
	}
}

func TestAMissingNameDoesNotLeaveAGapInTheSentence(t *testing.T) {
	t.Parallel()

	body := Body(TypeMedicationReminder, Subject{Timezone: "Asia/Kolkata"},
		time.Date(2026, 3, 1, 8, 0, 0, 0, kolkata), time.Date(2026, 3, 1, 7, 45, 0, 0, kolkata))

	if strings.Contains(body, "  ") || strings.Contains(body, " for  ") {
		t.Errorf("body %q has a hole where the name should be", body)
	}
	if !strings.Contains(body, "08:00") {
		t.Errorf("body %q lost the time", body)
	}
}

func TestAnActivityBodySaysWhoDidItWithoutSayingWhat(t *testing.T) {
	t.Parallel()

	subject := Subject{Name: "Amma", Timezone: "Asia/Kolkata"}
	body := ActivityBody(ActivityMedicationRecorded, "Priya", subject)

	if !strings.Contains(body, "Priya") {
		t.Errorf("body %q does not say who acted", body)
	}
	if !strings.Contains(body, "Amma") {
		t.Errorf("body %q does not say whose care it was", body)
	}
}
