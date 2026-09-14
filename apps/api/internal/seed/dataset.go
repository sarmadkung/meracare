package seed

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/genxcare/api/internal/care"
)

/*
The rows, worked out before anything is written.

Building and writing are separate so the interesting half — what a seeded day
actually looks like — can be read and tested without a database. Everything
here refers to people by persona key rather than by user id: the id is whatever
the database already holds for that account, and only the writer knows it.
*/

// Dataset is a whole seeded world, ready to insert.
type Dataset struct {
	Users         []UserRow
	Seniors       []SeniorRow
	Relationships []RelationshipRow
	Invitations   []InvitationRow
	Medications   []MedicationRow
	Schedules     []ScheduleRow
	Doses         []DoseRow
	TaskTemplates []TaskTemplateRow
	Tasks         []TaskRow
	Appointments  []AppointmentRow
	Notes         []NoteRow
	Messages      []MessageRow
	Events        []EventRow
	Notifications []NotificationRow
}

type UserRow struct {
	PersonaKey  string
	DisplayName string
	Email       string
	// AuthUserID is the Supabase identity. Derived for a persona who never
	// signs in; supplied by the account step for everybody else.
	AuthUserID uuid.UUID
}

type SeniorRow struct {
	ID          uuid.UUID
	CircleKey   string
	DisplayName string
	Timezone    string
	DateOfBirth string
	Phone       string
	Address     string
	Emergency   string
	SelfKey     string
	OwnerKey    string
	Archived    bool
}

type RelationshipRow struct {
	ID          uuid.UUID
	SeniorID    uuid.UUID
	PersonaKey  string
	Role        care.Role
	Permissions []string
	Status      string
	JoinedAt    time.Time
}

type InvitationRow struct {
	ID          uuid.UUID
	SeniorID    uuid.UUID
	InviterKey  string
	Email       string
	Role        care.Role
	Permissions []string
	TokenHash   []byte
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

type MedicationRow struct {
	ID           uuid.UUID
	SeniorID     uuid.UUID
	CircleKey    string
	CreatedByKey string
	Name         string
	Dosage       string
	Form         string
	Instructions string
	Notes        string
}

type ScheduleRow struct {
	ID           uuid.UUID
	MedicationID uuid.UUID
	SeniorID     uuid.UUID
	Rule         string
	// Wall-clock time in the senior's zone, as the column stores it.
	At time.Time
}

type DoseRow struct {
	ID           uuid.UUID
	ScheduleID   uuid.UUID
	MedicationID uuid.UUID
	SeniorID     uuid.UUID
	CircleKey    string
	CreatedByKey string
	Name         string
	Dosage       string
	ScheduledFor time.Time
	Status       string
	ActedAt      time.Time
	ActorKey     string
}

type TaskTemplateRow struct {
	ID           uuid.UUID
	SeniorID     uuid.UUID
	CreatedByKey string
	Title        string
	Description  string
	AssignedKey  string
	Rule         string
	At           time.Time
}

type TaskRow struct {
	ID           uuid.UUID
	TemplateID   uuid.UUID
	SeniorID     uuid.UUID
	CircleKey    string
	CreatedByKey string
	Title        string
	Description  string
	AssignedKey  string
	ScheduledFor time.Time
	Status       string
	ActedAt      time.Time
	ActorKey     string
}

type AppointmentRow struct {
	ID           uuid.UUID
	SeniorID     uuid.UUID
	CircleKey    string
	CreatedByKey string
	Title        string
	Kind         string
	Provider     string
	Location     string
	Notes        string
	AssignedKey  string
	ScheduledAt  time.Time
	EndsAt       time.Time
	Status       string
	ActedAt      time.Time
	ActorKey     string
}

type NoteRow struct {
	ID        uuid.UUID
	SeniorID  uuid.UUID
	AuthorKey string
	Content   string
	CreatedAt time.Time
}

type MessageRow struct {
	ID        uuid.UUID
	SeniorID  uuid.UUID
	SenderKey string
	Content   string
	CreatedAt time.Time
}

type EventRow struct {
	ID         uuid.UUID
	SeniorID   uuid.UUID
	ActorKey   string
	Type       string
	EntityType string
	EntityID   uuid.UUID
	Metadata   map[string]string
	OccurredAt time.Time
}

type NotificationRow struct {
	ID           uuid.UUID
	RecipientKey string
	SeniorID     uuid.UUID
	Type         string
	Title        string
	Body         string
	EntityType   string
	EntityID     uuid.UUID
	ScheduledFor time.Time
	DedupeKey    string
	Read         bool
}

// medicine is one entry in the fictional formulary the circles draw from.
type medicine struct {
	name, dosage, form, instructions string
}

var formulary = []medicine{
	{"Metformin", "500 mg", "tablet", "With food"},
	{"Amlodipine", "5 mg", "tablet", "In the morning"},
	{"Atorvastatin", "20 mg", "tablet", "At night"},
	{"Levothyroxine", "50 mcg", "tablet", "Before breakfast"},
	{"Salbutamol", "two puffs", "inhaler", "If short of breath"},
	{"Furosemide", "40 mg", "tablet", "In the morning"},
	{"Gliclazide", "80 mg", "tablet", "With breakfast"},
	{"Bisoprolol", "2.5 mg", "tablet", "In the morning"},
	{"Calcium and vitamin D", "1 tablet", "tablet", "After lunch"},
}

// Build works out every row the plan implies, as of now.
func Build(plan Plan, now time.Time) (Dataset, error) {
	if err := plan.Validate(); err != nil {
		return Dataset{}, err
	}

	data := Dataset{}

	for _, persona := range plan.Personas {
		row := UserRow{PersonaKey: persona.Key, DisplayName: persona.Name, Email: persona.Email}
		if !persona.SignsIn {
			// Nobody will ever present a token for this identity. It exists so
			// the row can satisfy the not-null identity column, and it is
			// derived so a re-run finds the same person rather than a second.
			row.AuthUserID = ID("auth", persona.Key)
		}
		data.Users = append(data.Users, row)
	}

	for index, circle := range plan.Circles {
		if err := buildCircle(&data, circle, index, now); err != nil {
			return Dataset{}, err
		}
	}

	return data, nil
}

func buildCircle(data *Dataset, circle Circle, index int, now time.Time) error {
	zone, err := time.LoadLocation(circle.Timezone)
	if err != nil {
		return fmt.Errorf("seed: circle %q has unknown timezone %q: %w", circle.Key, circle.Timezone, err)
	}

	seniorID := ID("senior", circle.Key)
	local := now.In(zone)
	day := shapeOfDay(local)

	data.Seniors = append(data.Seniors, SeniorRow{
		ID: seniorID, CircleKey: circle.Key, DisplayName: circle.Name,
		Timezone: circle.Timezone, DateOfBirth: circle.DateOfBirth,
		Phone: circle.Phone, Address: circle.Address, Emergency: circle.Emergency,
		SelfKey: circle.SelfKey, OwnerKey: circle.OwnerKey, Archived: circle.Archived,
	})

	var active []string

	for i, member := range circle.Members {
		joined := now.AddDate(0, 0, -90+i*7)
		data.Relationships = append(data.Relationships, RelationshipRow{
			ID:          ID("relationship", circle.Key+":"+member.PersonaKey),
			SeniorID:    seniorID,
			PersonaKey:  member.PersonaKey,
			Role:        member.Role,
			Permissions: care.Normalise(care.DefaultPermissions(member.Role)).Strings(),
			Status:      member.Status,
			JoinedAt:    joined,
		})

		if member.Status == "active" {
			active = append(active, member.PersonaKey)
		}

		eventType := "MEMBER_JOINED"
		if member.Status == "revoked" {
			eventType = "MEMBER_REVOKED"
		}
		data.Events = append(data.Events, EventRow{
			ID:       ID("event", circle.Key+":member:"+member.PersonaKey),
			SeniorID: seniorID, ActorKey: circle.OwnerKey,
			Type: eventType, EntityType: "relationship",
			EntityID:   ID("relationship", circle.Key+":"+member.PersonaKey),
			Metadata:   map[string]string{"role": string(member.Role)},
			OccurredAt: joined,
		})
	}

	if len(active) == 0 {
		return fmt.Errorf("seed: circle %q has nobody left in it", circle.Key)
	}

	// Rotates so two circles are not identical, and so the person switching
	// between them can tell which one they are looking at.
	pick := func(offset int) string { return active[(index+offset)%len(active)] }

	buildInvitations(data, circle, seniorID, now)
	buildMedication(data, circle, seniorID, index, zone, now, day, pick)
	buildTasks(data, circle, seniorID, zone, now, day, pick)
	buildAppointments(data, circle, seniorID, local, now, pick)
	buildTalk(data, circle, seniorID, now, active, pick)
	buildNotifications(data, circle, seniorID, active, day)

	return nil
}

/*
The four moments a useful day needs.

They are placed relative to now rather than at fixed hours, so the day has the
same shape whenever the seeder is run: something already done, something that
slipped, one thing to do next, and an evening still ahead. Fixed hours would
mean a run at four in the afternoon produced a morning with nothing missed and
a screen that never shows its attention state.

The proportions are of the day either side of now, so nothing lands on
tomorrow. Run within the first minutes of a senior's local day the past is
correspondingly thin, which is true rather than a defect.
*/
type dayShape struct{ done, slipped, soon, later time.Time }

func shapeOfDay(local time.Time) dayShape {
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	end := start.Add(23*time.Hour + 45*time.Minute)

	behind := local.Sub(start)
	ahead := end.Sub(local)

	round := func(t time.Time) time.Time { return t.Truncate(5 * time.Minute) }

	return dayShape{
		done:    round(local.Add(-behind * 3 / 5)),
		slipped: round(local.Add(-behind / 5)),
		soon:    round(local.Add(ahead / 12)),
		later:   round(local.Add(ahead / 2)),
	}
}
