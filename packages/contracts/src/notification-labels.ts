import { timeInTimezone } from './datetime';
import type { Reminder, ReminderType } from './notification';

/**
 * The words a notification says.
 *
 * One place, as with every other user-visible sentence in GenxCare. It matters
 * more here than elsewhere: a notification appears on a locked phone, in front
 * of whoever happens to be holding it, so the wording is a privacy decision
 * rather than a copy decision (plans/phase8.md §§17, 47).
 */

/**
 * The title, which is all a glanced-at notification really shows.
 *
 * Exactly the wording plans/phase8.md §47 asks for. Nothing here names a
 * medicine, a dose, a condition, or a task — "Take your 500mg Metformin for
 * diabetes" is the example the brief gives of what not to do, and the reason is
 * that a lock screen has no idea who is looking at it.
 */
const TITLES: Record<ReminderType, string> = {
  MEDICATION_REMINDER: 'Medication reminder',
  TASK_REMINDER: 'Care task reminder',
  APPOINTMENT_REMINDER: 'Upcoming appointment',
};

/** The notification's title. */
export function reminderTitle(reminder: Reminder): string {
  return TITLES[reminder.type];
}

/**
 * The notification's second line.
 *
 * Who it is about and when — which is the minimum a caregiver looking after two
 * people needs in order to know whether to act, and the maximum that can be
 * shown without saying anything medical. The time is rendered in the senior's
 * timezone, so a daughter in London reads her mother's 08:00 rather than her
 * own (plans/phase8.md §32).
 */
export function reminderBody(reminder: Reminder): string {
  const at = timeInTimezone(reminder.dueAt, reminder.seniorTimezone);

  switch (reminder.type) {
    case 'MEDICATION_REMINDER':
      return `A dose is due for ${reminder.seniorName} at ${at}.`;
    case 'TASK_REMINDER':
      return `Something is due for ${reminder.seniorName} at ${at}.`;
    case 'APPOINTMENT_REMINDER':
      return `${reminder.seniorName} has an appointment at ${at}.`;
  }
}

/**
 * The small amount of data a notification carries so that tapping it can open
 * the right screen.
 *
 * Only identifiers. Everything the screen shows is fetched afterwards, under
 * the caller's own authorization, which means a notification that outlives
 * somebody's access to a senior opens nothing (plans/phase8.md §§16, 23).
 */
export interface ReminderPayload {
  reminderId: string;
  type: ReminderType;
  seniorId: string;
  entityType: Reminder['entityType'];
  entityId: string;
}

/** Builds the payload attached to a scheduled notification. */
export function reminderPayload(reminder: Reminder): ReminderPayload {
  return {
    reminderId: reminder.id,
    type: reminder.type,
    seniorId: reminder.seniorId,
    entityType: reminder.entityType,
    entityId: reminder.entityId,
  };
}

/**
 * Reads a payload back off a notification the user tapped.
 *
 * Returns null rather than throwing for anything unrecognised. A notification
 * can outlive the app version that scheduled it — an old one may still be
 * pending after an update that changed this shape — and a crash on opening a
 * stale notification would be a worse bug than a tap that merely does nothing
 * (plans/phase8.md §37).
 */
export function readReminderPayload(data: unknown): ReminderPayload | null {
  if (typeof data !== 'object' || data === null) return null;

  const candidate = data as Record<string, unknown>;
  const strings = ['reminderId', 'type', 'seniorId', 'entityType', 'entityId'] as const;

  for (const key of strings) {
    if (typeof candidate[key] !== 'string' || candidate[key] === '') return null;
  }

  return {
    reminderId: candidate.reminderId as string,
    type: candidate.type as ReminderType,
    seniorId: candidate.seniorId as string,
    entityType: candidate.entityType as Reminder['entityType'],
    entityId: candidate.entityId as string,
  };
}
