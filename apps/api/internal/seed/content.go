package seed

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/care"
)

/*
What fills a circle.

Each builder answers one question about the day: what is being taken, what has
to be done, what is booked, what people have said to each other, and what the
app told them about it.
*/

// InviteToken is the raw token for a seeded invitation.
//
// Derived rather than random so the seeder can print a working join link: an
// invitation nobody can accept only ever shows one half of the flow.
func InviteToken(circleKey, email string) string {
	digest := sha256.Sum256([]byte("meracare-seed-invite:" + circleKey + ":" + email))
	return hex.EncodeToString(digest[:])
}

func buildInvitations(data *Dataset, circle Circle, seniorID uuid.UUID, now time.Time) {
	for _, invitation := range circle.Invitations {
		token := InviteToken(circle.Key, invitation.Email)
		hash := sha256.Sum256([]byte(token))

		id := ID("invitation", circle.Key+":"+invitation.Email)
		created := now.AddDate(0, 0, -2)

		data.Invitations = append(data.Invitations, InvitationRow{
			ID: id, SeniorID: seniorID, InviterKey: circle.OwnerKey,
			Email: invitation.Email, Role: invitation.Role,
			Permissions: care.Normalise(care.DefaultPermissions(invitation.Role)).Strings(),
			TokenHash:   hash[:], ExpiresAt: now.AddDate(0, 0, 5), CreatedAt: created,
		})

		data.Events = append(data.Events, EventRow{
			ID: ID("event", circle.Key+":invite:"+invitation.Email), SeniorID: seniorID,
			ActorKey: circle.OwnerKey, Type: "MEMBER_INVITED", EntityType: "invitation",
			EntityID: id, Metadata: map[string]string{"email": invitation.Email},
			OccurredAt: created,
		})
	}
}

/*
Medicine, and the doses it has already produced.

Three medicines, four schedules: one taken twice a day, one that is the thing
that slipped this morning, and one due shortly. Yesterday is filled in as taken
and tomorrow as pending, so the history screen has a history and the coming
days are not blank.
*/
func buildMedication(
	data *Dataset,
	circle Circle,
	seniorID uuid.UUID,
	index int,
	zone *time.Location,
	now time.Time,
	day dayShape,
	pick func(int) string,
) {
	type slot struct {
		medicine  int
		at        time.Time
		todayDone bool
	}

	slots := []slot{
		{medicine: 0, at: day.done, todayDone: true},
		{medicine: 0, at: day.later},
		{medicine: 1, at: day.slipped},
		{medicine: 2, at: day.soon},
	}

	medications := make([]MedicationRow, 3)
	for i := range medications {
		entry := formulary[(index*3+i)%len(formulary)]
		id := ID("medication", fmt.Sprintf("%s:%d", circle.Key, i))

		medications[i] = MedicationRow{
			ID: id, SeniorID: seniorID, CircleKey: circle.Key,
			CreatedByKey: circle.OwnerKey, Name: entry.name, Dosage: entry.dosage,
			Form: entry.form, Instructions: entry.instructions,
		}
		data.Medications = append(data.Medications, medications[i])

		data.Events = append(data.Events, EventRow{
			ID: ID("event", fmt.Sprintf("%s:med:%d", circle.Key, i)), SeniorID: seniorID,
			ActorKey: circle.OwnerKey, Type: "MEDICATION_CREATED", EntityType: "medication",
			EntityID: id, Metadata: map[string]string{"name": entry.name, "dosage": entry.dosage},
			OccurredAt: now.AddDate(0, 0, -60+i),
		})
	}

	for i, s := range slots {
		medication := medications[s.medicine]
		scheduleID := ID("schedule", fmt.Sprintf("%s:%d", circle.Key, i))

		data.Schedules = append(data.Schedules, ScheduleRow{
			ID: scheduleID, MedicationID: medication.ID, SeniorID: seniorID,
			Rule: "FREQ=DAILY", At: s.at,
		})

		// Yesterday taken, today as the day's shape says, tomorrow still to come.
		for _, offset := range []int{-1, 0, 1} {
			at := s.at.AddDate(0, 0, offset)

			status, actor := "pending", ""
			acted := time.Time{}

			if offset < 0 || (offset == 0 && s.todayDone) {
				status, actor = "taken", pick(i)
				acted = at.Add(time.Duration(3+i) * time.Minute)
			}

			doseID := ID("dose", fmt.Sprintf("%s:%d:%d", circle.Key, i, offset))
			data.Doses = append(data.Doses, DoseRow{
				ID: doseID, ScheduleID: scheduleID, MedicationID: medication.ID,
				SeniorID: seniorID, CircleKey: circle.Key, CreatedByKey: circle.OwnerKey,
				Name: medication.Name, Dosage: medication.Dosage,
				ScheduledFor: at.In(zone), Status: status, ActedAt: acted, ActorKey: actor,
			})

			if status == "taken" {
				data.Events = append(data.Events, EventRow{
					ID:       ID("event", fmt.Sprintf("%s:dose:%d:%d", circle.Key, i, offset)),
					SeniorID: seniorID, ActorKey: actor, Type: "MEDICATION_TAKEN",
					EntityType: "medication", EntityID: doseID,
					Metadata:   map[string]string{"name": medication.Name, "dosage": medication.Dosage},
					OccurredAt: acted,
				})
			}
		}
	}
}

/*
Tasks: one done, one that has slipped, one still ahead and assigned.

The assigned one is what puts a "Yours" tag on somebody's day. Without it the
whole assignment path is invisible, because a family member caring alone never
assigns anything to themselves.
*/
func buildTasks(
	data *Dataset,
	circle Circle,
	seniorID uuid.UUID,
	zone *time.Location,
	now time.Time,
	day dayShape,
	pick func(int) string,
) {
	templateID := ID("task-template", circle.Key)

	data.TaskTemplates = append(data.TaskTemplates, TaskTemplateRow{
		ID: templateID, SeniorID: seniorID, CreatedByKey: circle.OwnerKey,
		Title: "Morning walk", Description: "Twenty minutes, weather permitting.",
		Rule: "FREQ=DAILY", At: day.done,
	})

	for _, offset := range []int{-1, 0} {
		at := day.done.AddDate(0, 0, offset)
		data.Tasks = append(data.Tasks, TaskRow{
			ID:         ID("task", fmt.Sprintf("%s:walk:%d", circle.Key, offset)),
			TemplateID: templateID, SeniorID: seniorID, CircleKey: circle.Key,
			CreatedByKey: circle.OwnerKey, Title: "Morning walk",
			Description:  "Twenty minutes, weather permitting.",
			ScheduledFor: at.In(zone), Status: "completed",
			ActedAt: at.Add(35 * time.Minute), ActorKey: pick(0),
		})
	}

	oneOffs := []struct {
		key, title, description, assigned, status string
		at                                        time.Time
	}{
		{"pressure", "Blood pressure check", "Log the reading in the notes.", "", "pending", day.slipped},
		{"physio", "Physiotherapy exercises", "The sheet from the clinic.", pick(1), "pending", day.later},
		{"shopping", "Collect repeat prescription", "The pharmacy on the corner.", pick(0), "pending", day.later.AddDate(0, 0, 1)},
	}

	for _, task := range oneOffs {
		data.Tasks = append(data.Tasks, TaskRow{
			ID: ID("task", circle.Key+":"+task.key), SeniorID: seniorID, CircleKey: circle.Key,
			CreatedByKey: circle.OwnerKey, Title: task.title, Description: task.description,
			AssignedKey: task.assigned, ScheduledFor: task.at.In(zone), Status: task.status,
		})
	}

	data.Events = append(data.Events, EventRow{
		ID: ID("event", circle.Key+":task:walk"), SeniorID: seniorID, ActorKey: pick(0),
		Type: "TASK_COMPLETED", EntityType: "task",
		EntityID:   ID("task", circle.Key+":walk:0"),
		Metadata:   map[string]string{"title": "Morning walk"},
		OccurredAt: day.done.Add(35 * time.Minute),
	})
}

func buildAppointments(
	data *Dataset,
	circle Circle,
	seniorID uuid.UUID,
	local time.Time,
	now time.Time,
	pick func(int) string,
) {
	at := func(days, hour, minute int) time.Time {
		d := local.AddDate(0, 0, days)
		return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, local.Location())
	}

	entries := []struct {
		key, title, kind, provider, location, status, assigned string
		start                                                  time.Time
		hours                                                  int
	}{
		{"cardiology", "Cardiology follow-up", "doctor_visit", "Dr Farooq", "Shifa clinic, room 3", "scheduled", pick(0), at(3, 11, 0), 1},
		{"bloods", "Blood tests", "laboratory", "Chughtai Lab", "Main branch", "completed", pick(1), at(-8, 10, 30), 1},
		{"physio", "Physiotherapy assessment", "therapy", "Ms Kausar", "Community centre", "cancelled", "", at(6, 15, 0), 2},
	}

	for _, entry := range entries {
		id := ID("appointment", circle.Key+":"+entry.key)

		row := AppointmentRow{
			ID: id, SeniorID: seniorID, CircleKey: circle.Key, CreatedByKey: circle.OwnerKey,
			Title: entry.title, Kind: entry.kind, Provider: entry.provider, Location: entry.location,
			AssignedKey: entry.assigned, ScheduledAt: entry.start,
			EndsAt: entry.start.Add(time.Duration(entry.hours) * time.Hour), Status: entry.status,
		}

		switch entry.status {
		case "completed":
			row.ActedAt, row.ActorKey = entry.start.Add(90*time.Minute), pick(1)
			data.Events = append(data.Events, EventRow{
				ID: ID("event", circle.Key+":appt:"+entry.key), SeniorID: seniorID,
				ActorKey: pick(1), Type: "APPOINTMENT_COMPLETED", EntityType: "appointment",
				EntityID: id, Metadata: map[string]string{"title": entry.title},
				OccurredAt: row.ActedAt,
			})
		case "cancelled":
			row.ActedAt, row.ActorKey = now.AddDate(0, 0, -1), pick(0)
			data.Events = append(data.Events, EventRow{
				ID: ID("event", circle.Key+":appt:"+entry.key), SeniorID: seniorID,
				ActorKey: pick(0), Type: "APPOINTMENT_CANCELLED", EntityType: "appointment",
				EntityID: id, Metadata: map[string]string{"title": entry.title},
				OccurredAt: row.ActedAt,
			})
		}

		data.Appointments = append(data.Appointments, row)
	}
}

// buildTalk fills the notes and the message thread.
//
// A circle of one still gets notes — a senior keeps their own record — but a
// conversation needs somebody to have it with.
func buildTalk(
	data *Dataset,
	circle Circle,
	seniorID uuid.UUID,
	now time.Time,
	active []string,
	pick func(int) string,
) {
	notes := []struct {
		key, content string
		hoursAgo     int
	}{
		{"appetite", "Ate very little at lunch again. Worth mentioning at the cardiology follow-up.", 6},
		{"sleep", "Slept through the night without waking. First time this week.", 30},
	}

	for i, note := range notes {
		id := ID("note", circle.Key+":"+note.key)
		occurred := now.Add(-time.Duration(note.hoursAgo) * time.Hour)

		data.Notes = append(data.Notes, NoteRow{
			ID: id, SeniorID: seniorID, AuthorKey: pick(i), Content: note.content,
			CreatedAt: occurred,
		})
		data.Events = append(data.Events, EventRow{
			ID: ID("event", circle.Key+":note:"+note.key), SeniorID: seniorID,
			ActorKey: pick(i), Type: "NOTE_ADDED", EntityType: "note", EntityID: id,
			Metadata: map[string]string{}, OccurredAt: occurred,
		})
	}

	if len(active) < 2 {
		return
	}

	thread := []string{
		"I have moved the cardiology appointment to Thursday morning — the clinic called.",
		"Noted. I can take her, I am off that day.",
		"Thank you. The blood pressure reading this morning was a little high, it is in the notes.",
	}

	for i, line := range thread {
		data.Messages = append(data.Messages, MessageRow{
			ID: ID("message", fmt.Sprintf("%s:%d", circle.Key, i)), SeniorID: seniorID,
			SenderKey: active[i%len(active)], Content: line,
			CreatedAt: now.Add(-time.Duration(len(thread)-i) * 40 * time.Minute),
		})
	}
}

// buildNotifications fills each member's inbox.
//
// Two unread and one already read, because an inbox where everything is unread
// looks the same as an inbox where nothing is.
func buildNotifications(
	data *Dataset,
	circle Circle,
	seniorID uuid.UUID,
	active []string,
	day dayShape,
) {
	for _, recipient := range active {
		entries := []struct {
			key, kind, title, body, entityType string
			entity                             uuid.UUID
			at                                 time.Time
			read                               bool
		}{
			{
				"missed", "MEDICATION_MISSED", "Dose not recorded",
				fmt.Sprintf("%s's %s has not been recorded.", circle.Name, "morning dose"),
				"medication_dose", ID("dose", circle.Key+":2:0"), day.slipped, false,
			},
			{
				"overdue", "TASK_OVERDUE", "Blood pressure check is overdue",
				fmt.Sprintf("It was due earlier today for %s.", circle.Name),
				"task_instance", ID("task", circle.Key+":pressure"), day.slipped, false,
			},
			{
				"activity", "CARE_ACTIVITY", "Morning walk completed",
				fmt.Sprintf("Somebody marked it done for %s.", circle.Name),
				"task_instance", ID("task", circle.Key+":walk:0"), day.done, true,
			},
		}

		for _, entry := range entries {
			data.Notifications = append(data.Notifications, NotificationRow{
				ID:           ID("notification", circle.Key+":"+recipient+":"+entry.key),
				RecipientKey: recipient, SeniorID: seniorID, Type: entry.kind,
				Title: entry.title, Body: entry.body,
				EntityType: entry.entityType, EntityID: entry.entity,
				ScheduledFor: entry.at,
				DedupeKey:    "seed:" + circle.Key + ":" + recipient + ":" + entry.key,
				Read:         entry.read,
			})
		}
	}
}
