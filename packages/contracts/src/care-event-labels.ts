import type { CareEvent, CareEventType } from './care-event';
import { dateInTimezone, dateKeyInTimezone, timeInTimezone } from './datetime';

/**
 * Plain-language wording for the activity timeline.
 *
 * One place, not one per screen. Every event's wording is decided here, so a
 * timeline entry and a future notification saying the same thing cannot drift
 * apart, and adding an event type is one edit rather than a search
 * (plans/phase7.md §15).
 *
 * The server sends `TASK_COMPLETED`; nothing shows it. An older adult reads
 * "Sarah marked Morning walk as done" (plans/phase7.md §14).
 */

/**
 * The part of care an event belongs to.
 *
 * Shown as a small caption on each row. It fills the role an icon would, in
 * words: the brand icon set is still outstanding, and a word is legible to
 * somebody who cannot make out a 16-pixel glyph anyway. Meaning never rests on
 * colour alone (plans/phase7.md §31).
 */
export type CareEventCategory = 'Task' | 'Medication' | 'Appointment' | 'Care circle' | 'Note';

const CATEGORIES: Record<CareEventType, CareEventCategory> = {
  MEMBER_INVITED: 'Care circle',
  MEMBER_JOINED: 'Care circle',
  MEMBER_REVOKED: 'Care circle',
  TASK_CREATED: 'Task',
  TASK_COMPLETED: 'Task',
  TASK_SKIPPED: 'Task',
  TASK_MISSED: 'Task',
  MEDICATION_CREATED: 'Medication',
  MEDICATION_TAKEN: 'Medication',
  MEDICATION_SKIPPED: 'Medication',
  MEDICATION_MISSED: 'Medication',
  APPOINTMENT_CREATED: 'Appointment',
  APPOINTMENT_COMPLETED: 'Appointment',
  APPOINTMENT_CANCELLED: 'Appointment',
  NOTE_ADDED: 'Note',
};

export function careEventCategory(type: CareEventType): CareEventCategory {
  return CATEGORIES[type];
}

/**
 * The tone a row should be shown in, for callers that also use colour.
 *
 * Most of a timeline is `neutral` on purpose. A feed where every row is
 * coloured is a feed where nothing stands out, and care activity is mostly
 * ordinary — somebody did what they said they would (plans/phase7.md §31).
 */
export type CareEventTone = 'neutral' | 'positive' | 'muted';

const TONES: Record<CareEventType, CareEventTone> = {
  MEMBER_INVITED: 'neutral',
  MEMBER_JOINED: 'positive',
  MEMBER_REVOKED: 'muted',
  TASK_CREATED: 'neutral',
  TASK_COMPLETED: 'positive',
  TASK_SKIPPED: 'muted',
  TASK_MISSED: 'muted',
  MEDICATION_CREATED: 'neutral',
  MEDICATION_TAKEN: 'positive',
  MEDICATION_SKIPPED: 'muted',
  MEDICATION_MISSED: 'muted',
  APPOINTMENT_CREATED: 'neutral',
  APPOINTMENT_COMPLETED: 'positive',
  APPOINTMENT_CANCELLED: 'muted',
  NOTE_ADDED: 'neutral',
};

export function careEventTone(type: CareEventType): CareEventTone {
  return TONES[type];
}

/** What the event was about: the task's title, the medicine's name. */
export function careEventSubject(event: Pick<CareEvent, 'type' | 'metadata'>): string {
  const { metadata } = event;

  switch (careEventCategory(event.type)) {
    case 'Task':
      return metadata.taskTitle ?? 'a task';
    case 'Medication':
      return metadata.medicationName ?? 'a medication';
    case 'Appointment':
      return metadata.appointmentTitle ?? 'an appointment';
    case 'Care circle':
      return metadata.memberName ?? 'somebody';
    case 'Note':
      return 'a note';
  }
}

/**
 * What happened, as a sentence.
 *
 * `actorName` resolves the actor; a caller that cannot name them should pass
 * "Somebody", which is what a revoked member's entries read as once they are
 * no longer in the circle to look up.
 */
export function careEventDescription(
  event: Pick<CareEvent, 'type' | 'metadata'>,
  actorName: string,
): string {
  const subject = careEventSubject(event);
  const { metadata } = event;

  switch (event.type) {
    case 'TASK_CREATED':
      return `${actorName} added ${subject}`;
    case 'TASK_COMPLETED':
      return `${actorName} marked ${subject} as done`;
    case 'TASK_SKIPPED':
      return `${actorName} marked ${subject} as not needed`;
    case 'TASK_MISSED':
      return `${subject} was not done`;

    case 'MEDICATION_CREATED':
      return `${actorName} added ${subject}${metadata.dosage ? `, ${metadata.dosage}` : ''}`;
    case 'MEDICATION_TAKEN':
      return `${actorName} recorded ${subject} as taken`;
    case 'MEDICATION_SKIPPED':
      return `${actorName} recorded ${subject} as not taken`;
    case 'MEDICATION_MISSED':
      return `${subject} was not taken`;

    case 'APPOINTMENT_CREATED':
      return `${actorName} added ${subject}`;
    case 'APPOINTMENT_COMPLETED':
      return `${actorName} marked ${subject} as attended`;
    case 'APPOINTMENT_CANCELLED':
      return `${actorName} cancelled ${subject}`;

    case 'MEMBER_INVITED':
      return `${actorName} invited ${subject}`;
    case 'MEMBER_JOINED':
      return `${subject} joined the care circle`;
    case 'MEMBER_REVOKED':
      return actorName === subject
        ? `${subject} left the care circle`
        : `${actorName} removed ${subject} from the care circle`;

    case 'NOTE_ADDED':
      return `${actorName} added ${subject}`;
  }
}

/** The time an event happened, in the senior's timezone: "10:42". */
export function careEventTimeLabel(event: Pick<CareEvent, 'occurredAt'>, timezone: string): string {
  return timeInTimezone(event.occurredAt, timezone);
}

/**
 * The heading a day's events sit under: "Today", "Yesterday", or the date.
 *
 * Worked out in the senior's timezone, so an event at half past midnight in
 * Karachi is filed under that day there rather than under the previous one
 * (plans/phase7.md §16).
 */
export function dayHeading(instant: string, timezone: string, now: Date = new Date()): string {
  const day = dateKeyInTimezone(instant, timezone);
  const today = dateKeyInTimezone(now.toISOString(), timezone);

  if (day === today) return 'Today';

  const yesterday = dateKeyInTimezone(
    new Date(now.getTime() - 24 * 60 * 60 * 1000).toISOString(),
    timezone,
  );
  if (day === yesterday) return 'Yesterday';

  return dateInTimezone(instant, timezone);
}

/** One day's events, under the heading they belong to. */
export interface CareEventDay {
  /** The senior's local `YYYY-MM-DD`, stable enough to be a list key. */
  key: string;
  heading: string;
  events: CareEvent[];
}

/**
 * Groups a newest-first list of events into newest-first days.
 *
 * The order the server sent is preserved rather than re-sorted: it is already
 * the authority on ordering, and sorting again in the client would be a second
 * opinion that could disagree at a page boundary (plans/phase7.md §13).
 */
export function groupByDay(
  events: CareEvent[],
  timezone: string,
  now: Date = new Date(),
): CareEventDay[] {
  const days: CareEventDay[] = [];

  for (const event of events) {
    const key = dateKeyInTimezone(event.occurredAt, timezone);
    const current = days[days.length - 1];

    if (current !== undefined && current.key === key) {
      current.events.push(event);
      continue;
    }

    days.push({
      key,
      heading: dayHeading(event.occurredAt, timezone, now),
      events: [event],
    });
  }

  return days;
}
