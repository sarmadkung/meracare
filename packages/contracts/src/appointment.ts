/**
 * Appointments: where somebody has to be, who is taking them, and whether they
 * got there.
 *
 * Mirrors internal/appointments in the Go API and docs/03-domain-model.md's
 * Appointment. An appointment is deliberately not a care task: a task is a
 * routine the circle repeats, an appointment is a single commitment at a place
 * that somebody booked (plans/phase6.md, objective).
 *
 * This is care coordination, not clinical software. The app records that a
 * visit is on Thursday; nothing here reasons about whether the visit is needed
 * (plans/phase6.md §28).
 */

import type { CursorPage } from './pagination';

/**
 * The state of one appointment.
 *
 * Exactly the vocabulary in docs/03, and no more. There is deliberately no
 * derived status here, unlike a task's `overdue` or a dose's `missed`: a dose
 * whose hour has passed really has been missed, but an appointment whose hour
 * has passed has not become anything — nobody knows yet whether the person
 * went. Upcoming and past are read from the clock, which is a question about
 * the calendar rather than about the appointment.
 */
export const APPOINTMENT_STATUSES = ['scheduled', 'completed', 'cancelled'] as const;
export type AppointmentStatus = (typeof APPOINTMENT_STATUSES)[number];

/** Statuses that mean somebody has already decided the outcome. */
export const SETTLED_APPOINTMENT_STATUSES = ['completed', 'cancelled'] as const;

/** Reports whether the appointment can still be changed or settled. */
export function isAppointmentOpen(appointment: Pick<Appointment, 'status'>): boolean {
  return appointment.status === 'scheduled';
}

/** Reports whether the appointment has not started yet, as of `now`. */
export function isAppointmentUpcoming(
  appointment: Pick<Appointment, 'scheduledAt'>,
  now: Date = new Date(),
): boolean {
  return new Date(appointment.scheduledAt).getTime() > now.getTime();
}

/**
 * The sort of visit an appointment is.
 *
 * A short list, matching what the API accepts. Deliberately not a medical
 * taxonomy: GenxCare coordinates appointments and does not classify care
 * (plans/phase6.md §2).
 */
export const APPOINTMENT_KINDS = [
  'doctor_visit',
  'hospital_visit',
  'therapy',
  'laboratory',
  'care_meeting',
  'other',
] as const;
export type AppointmentKind = (typeof APPOINTMENT_KINDS)[number];

/** One commitment in a senior's calendar. */
export interface Appointment {
  id: string;
  seniorId: string;

  title: string;
  /** Null when nobody said what sort of visit it is. */
  kind: AppointmentKind | null;
  providerName: string | null;
  location: string | null;
  notes: string | null;

  /** The circle member taking them, when one has been named. */
  assignedUserId: string | null;

  /** ISO-8601 instant. Render it in the senior's timezone, not the device's. */
  scheduledAt: string;
  /**
   * ISO-8601 instant, or null when nobody said how long it would take. Because
   * both ends are instants, an appointment running past midnight needs no
   * special case.
   */
  endsAt: string | null;

  status: AppointmentStatus;

  completedAt: string | null;
  completedBy: string | null;
  cancelledAt: string | null;
  cancelledBy: string | null;

  /** Who booked it. Always the authenticated user, never a client's word. */
  createdBy: string;

  createdAt: string;
  updatedAt: string;
}

/** Which set of appointments to fetch. */
export const APPOINTMENT_SCOPES = ['today', 'upcoming', 'past'] as const;
export type AppointmentScope = (typeof APPOINTMENT_SCOPES)[number];

/** `POST /v1/seniors/{id}/appointments` request body. */
export interface CreateAppointmentRequest {
  title: string;
  kind?: AppointmentKind | null;
  providerName?: string;
  location?: string;
  notes?: string;
  assignedUserId?: string | null;
  /** ISO-8601 instant. */
  scheduledAt: string;
  endsAt?: string | null;
}

/** `PATCH /v1/appointments/{id}` request body. Absent fields are left unchanged. */
export interface UpdateAppointmentRequest {
  title?: string;
  kind?: AppointmentKind;
  /** Sent to remove the kind, which absence alone cannot express. */
  clearKind?: boolean;
  providerName?: string;
  location?: string;
  notes?: string;
  assignedUserId?: string;
  /** Sent to unassign, which absence alone cannot express. */
  clearAssignee?: boolean;
  scheduledAt?: string;
  endsAt?: string;
  clearEndsAt?: boolean;
}

/**
 * `GET /v1/seniors/{id}/appointments` response.
 *
 * One envelope for every view. `nextCursor` is null for the unpaged ones, so a
 * client reading the response never has to know which is which
 * (plans/phase6.md §14).
 */
export type AppointmentListResponse = CursorPage<Appointment>;
