package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/genxcare/api/internal/database"
)

/*
Putting the dataset into the database.

One transaction, and it opens by deleting the seniors a previous run created.
Everything a circle contains hangs off senior_profiles with ON DELETE CASCADE,
so that single statement removes the whole of the last run — and because the
identifiers are derived, it can only ever match rows this seeder wrote.

Nothing is truncated. On a shared database a TRUNCATE is indistinguishable from
erasing somebody's care record, and the tidy-up step of a development tool is
not a thing that should be able to do that.
*/

// Write applies the dataset, replacing anything a previous run left behind.
//
// users is the application user id for each persona, which the caller resolves
// first: an account may already exist, and its id is the database's to choose.
func Write(ctx context.Context, pool *database.Pool, data Dataset, users map[string]uuid.UUID) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("seed: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	seniorIDs := make([]uuid.UUID, 0, len(data.Seniors))
	for _, senior := range data.Seniors {
		seniorIDs = append(seniorIDs, senior.ID)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM senior_profiles WHERE id = ANY($1)`, seniorIDs); err != nil {
		return fmt.Errorf("seed: clear previous run: %w", err)
	}

	user := func(key string) any {
		if key == "" {
			return nil
		}
		id, ok := users[key]
		if !ok {
			return nil
		}
		return id
	}

	for _, senior := range data.Seniors {
		var archivedAt any
		var archivedBy any
		if senior.Archived {
			archivedAt = time.Now().AddDate(0, 0, -30)
			archivedBy = user(senior.OwnerKey)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO senior_profiles
				(id, user_id, created_by_user_id, display_name, date_of_birth, phone,
				 address, emergency_contact, timezone, archived_at, archived_by_user_id)
			VALUES ($1, $2, $3, $4, NULLIF($5, '')::date, NULLIF($6, ''), NULLIF($7, ''),
			        NULLIF($8, ''), $9, $10, $11)`,
			senior.ID, user(senior.SelfKey), user(senior.OwnerKey), senior.DisplayName,
			senior.DateOfBirth, senior.Phone, senior.Address, senior.Emergency,
			senior.Timezone, archivedAt, archivedBy,
		); err != nil {
			return fmt.Errorf("seed: senior %q: %w", senior.CircleKey, err)
		}
	}

	if err := writeCircleMembership(ctx, tx, data, user); err != nil {
		return err
	}
	if err := writeMedication(ctx, tx, data, user); err != nil {
		return err
	}
	if err := writeWork(ctx, tx, data, user); err != nil {
		return err
	}
	if err := writeRecord(ctx, tx, data, user); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("seed: commit: %w", err)
	}

	return nil
}

type resolver func(string) any

func writeCircleMembership(ctx context.Context, tx pgx.Tx, data Dataset, user resolver) error {
	for _, row := range data.Relationships {
		if _, err := tx.Exec(ctx, `
			INSERT INTO care_relationships (id, senior_id, user_id, role, permissions, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			row.ID, row.SeniorID, user(row.PersonaKey), string(row.Role),
			row.Permissions, row.Status, row.JoinedAt,
		); err != nil {
			return fmt.Errorf("seed: relationship for %q: %w", row.PersonaKey, err)
		}
	}

	for _, row := range data.Invitations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO invitations
				(id, senior_id, inviter_user_id, invitee_email, role, permissions,
				 token_hash, status, expires_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8, $9)`,
			row.ID, row.SeniorID, user(row.InviterKey), row.Email, string(row.Role),
			row.Permissions, row.TokenHash, row.ExpiresAt, row.CreatedAt,
		); err != nil {
			return fmt.Errorf("seed: invitation for %q: %w", row.Email, err)
		}
	}

	return nil
}

func writeMedication(ctx context.Context, tx pgx.Tx, data Dataset, user resolver) error {
	for _, row := range data.Medications {
		if _, err := tx.Exec(ctx, `
			INSERT INTO medications
				(id, senior_id, created_by_user_id, name, dosage, form, instructions, notes)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8)`,
			row.ID, row.SeniorID, user(row.CreatedByKey), row.Name, row.Dosage,
			row.Form, row.Instructions, row.Notes,
		); err != nil {
			return fmt.Errorf("seed: medication %q: %w", row.Name, err)
		}
	}

	for _, row := range data.Schedules {
		if _, err := tx.Exec(ctx, `
			INSERT INTO medication_schedules
				(id, medication_id, senior_id, recurrence_rule, scheduled_time)
			VALUES ($1, $2, $3, $4, $5::time)`,
			row.ID, row.MedicationID, row.SeniorID, row.Rule, row.At.Format("15:04:05"),
		); err != nil {
			return fmt.Errorf("seed: schedule: %w", err)
		}
	}

	for _, row := range data.Doses {
		var takenAt, takenBy, skippedAt, skippedBy any
		switch row.Status {
		case "taken":
			takenAt, takenBy = row.ActedAt, user(row.ActorKey)
		case "skipped":
			skippedAt, skippedBy = row.ActedAt, user(row.ActorKey)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO medication_instances
				(id, schedule_id, medication_id, senior_id, created_by_user_id, name, dosage,
				 scheduled_for, status, taken_at, taken_by, skipped_at, skipped_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			row.ID, row.ScheduleID, row.MedicationID, row.SeniorID, user(row.CreatedByKey),
			row.Name, row.Dosage, row.ScheduledFor, row.Status,
			takenAt, takenBy, skippedAt, skippedBy,
		); err != nil {
			return fmt.Errorf("seed: dose of %q: %w", row.Name, err)
		}
	}

	return nil
}

func writeWork(ctx context.Context, tx pgx.Tx, data Dataset, user resolver) error {
	for _, row := range data.TaskTemplates {
		if _, err := tx.Exec(ctx, `
			INSERT INTO care_task_templates
				(id, senior_id, created_by_user_id, title, description, assigned_user_id,
				 recurrence_rule, due_time)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::time)`,
			row.ID, row.SeniorID, user(row.CreatedByKey), row.Title, row.Description,
			user(row.AssignedKey), row.Rule, row.At.Format("15:04:05"),
		); err != nil {
			return fmt.Errorf("seed: task template %q: %w", row.Title, err)
		}
	}

	for _, row := range data.Tasks {
		var templateID any
		if row.TemplateID != uuid.Nil {
			templateID = row.TemplateID
		}

		var completedAt, completedBy, skippedAt, skippedBy any
		switch row.Status {
		case "completed":
			completedAt, completedBy = row.ActedAt, user(row.ActorKey)
		case "skipped":
			skippedAt, skippedBy = row.ActedAt, user(row.ActorKey)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO care_task_instances
				(id, template_id, senior_id, created_by_user_id, title, description,
				 assigned_user_id, scheduled_for, status, completed_at, completed_by,
				 skipped_at, skipped_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			row.ID, templateID, row.SeniorID, user(row.CreatedByKey), row.Title,
			row.Description, user(row.AssignedKey), row.ScheduledFor, row.Status,
			completedAt, completedBy, skippedAt, skippedBy,
		); err != nil {
			return fmt.Errorf("seed: task %q: %w", row.Title, err)
		}
	}

	for _, row := range data.Appointments {
		var completedAt, completedBy, cancelledAt, cancelledBy any
		switch row.Status {
		case "completed":
			completedAt, completedBy = row.ActedAt, user(row.ActorKey)
		case "cancelled":
			cancelledAt, cancelledBy = row.ActedAt, user(row.ActorKey)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO appointments
				(id, senior_id, created_by_user_id, title, kind, provider_name, location,
				 notes, assigned_user_id, scheduled_at, ends_at, status,
				 completed_at, completed_by, cancelled_at, cancelled_by)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			row.ID, row.SeniorID, user(row.CreatedByKey), row.Title, row.Kind,
			row.Provider, row.Location, row.Notes, user(row.AssignedKey),
			row.ScheduledAt, row.EndsAt, row.Status,
			completedAt, completedBy, cancelledAt, cancelledBy,
		); err != nil {
			return fmt.Errorf("seed: appointment %q: %w", row.Title, err)
		}
	}

	return nil
}

func writeRecord(ctx context.Context, tx pgx.Tx, data Dataset, user resolver) error {
	for _, row := range data.Notes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO care_notes (id, senior_id, author_user_id, content, created_at)
			VALUES ($1, $2, $3, $4, $5)`,
			row.ID, row.SeniorID, user(row.AuthorKey), row.Content, row.CreatedAt,
		); err != nil {
			return fmt.Errorf("seed: note: %w", err)
		}
	}

	for _, row := range data.Messages {
		if _, err := tx.Exec(ctx, `
			INSERT INTO messages (id, senior_id, sender_user_id, content, created_at)
			VALUES ($1, $2, $3, $4, $5)`,
			row.ID, row.SeniorID, user(row.SenderKey), row.Content, row.CreatedAt,
		); err != nil {
			return fmt.Errorf("seed: message: %w", err)
		}
	}

	for _, row := range data.Events {
		if _, err := tx.Exec(ctx, `
			INSERT INTO care_events
				(id, senior_id, actor_user_id, event_type, entity_type, entity_id,
				 metadata, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			row.ID, row.SeniorID, user(row.ActorKey), row.Type, row.EntityType,
			row.EntityID, row.Metadata, row.OccurredAt,
		); err != nil {
			return fmt.Errorf("seed: care event %q: %w", row.Type, err)
		}
	}

	for _, row := range data.Notifications {
		var readAt any
		if row.Read {
			readAt = row.ScheduledFor.Add(time.Minute)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO notifications
				(id, recipient_user_id, senior_id, notification_type, title, body,
				 entity_type, entity_id, scheduled_for, dedupe_key, delivery_status,
				 delivered_at, read_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'sent', $9, $11)`,
			row.ID, user(row.RecipientKey), row.SeniorID, row.Type, row.Title, row.Body,
			row.EntityType, row.EntityID, row.ScheduledFor, row.DedupeKey, readAt,
		); err != nil {
			return fmt.Errorf("seed: notification %q: %w", row.Title, err)
		}
	}

	return nil
}

// EnsureUsers inserts or updates the application user behind each persona and
// returns their ids.
//
// A persona whose account already exists keeps the id the database gave it, so
// re-seeding never orphans the data somebody has already been looking at.
func EnsureUsers(ctx context.Context, pool *database.Pool, rows []UserRow) (map[string]uuid.UUID, error) {
	ids := make(map[string]uuid.UUID, len(rows))

	for _, row := range rows {
		if row.AuthUserID == uuid.Nil {
			return nil, fmt.Errorf("seed: persona %q has no identity to attach to", row.PersonaKey)
		}

		var id uuid.UUID
		if err := pool.QueryRow(ctx, `
			INSERT INTO users (auth_user_id, email, display_name)
			VALUES ($1, NULLIF($2, ''), $3)
			ON CONFLICT (auth_user_id) DO UPDATE
				SET email = EXCLUDED.email, display_name = EXCLUDED.display_name
			RETURNING id`,
			row.AuthUserID, row.Email, row.DisplayName,
		).Scan(&id); err != nil {
			return nil, fmt.Errorf("seed: user %q: %w", row.PersonaKey, err)
		}

		ids[row.PersonaKey] = id
	}

	return ids, nil
}
