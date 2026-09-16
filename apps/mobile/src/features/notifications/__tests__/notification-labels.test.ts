import type { Reminder } from '@genxcare/contracts';
import {
  readReminderPayload,
  reminderBody,
  reminderPayload,
  reminderTitle,
} from '@genxcare/contracts';

/**
 * A notification appears on a locked phone, in front of whoever happens to be
 * holding it. What it may say is a privacy decision, so it is asserted rather
 * than left to review.
 */

function reminder(overrides: Partial<Reminder> = {}): Reminder {
  return {
    id: 'reminder-1',
    type: 'MEDICATION_REMINDER',
    seniorId: 'senior-1',
    seniorName: 'Amma',
    seniorTimezone: 'Asia/Karachi',
    entityType: 'medication_dose',
    entityId: 'dose-1',
    dueAt: '2026-08-19T03:00:00Z',
    fireAt: '2026-08-19T02:45:00Z',
    ...overrides,
  };
}

it('titles a medication reminder without naming the medicine', () => {
  expect(reminderTitle(reminder())).toBe('Medication reminder');
});

it('titles each kind of reminder in plain language', () => {
  expect(reminderTitle(reminder({ type: 'TASK_REMINDER' }))).toBe('Care task reminder');
  expect(reminderTitle(reminder({ type: 'APPOINTMENT_REMINDER' }))).toBe('Upcoming appointment');
});

it('shows the time in the senior’s timezone, not the reader’s', () => {
  // 03:00 UTC is 08:00 in Karachi. A daughter in London must read her mother's
  // eight o'clock, not her own four.
  expect(reminderBody(reminder())).toContain('08:00');
  expect(reminderBody(reminder())).toContain('Amma');
});

it('says nothing medical', () => {
  const body = reminderBody(reminder());

  for (const forbidden of ['Metformin', 'mg', 'dosage', 'diabetes', 'tablet']) {
    expect(body.toLowerCase()).not.toContain(forbidden.toLowerCase());
  }
});

it('carries only identifiers in its payload', () => {
  const payload = reminderPayload(reminder());

  expect(payload).toEqual({
    reminderId: 'reminder-1',
    type: 'MEDICATION_REMINDER',
    seniorId: 'senior-1',
    entityType: 'medication_dose',
    entityId: 'dose-1',
  });
});

it('reads its own payload back', () => {
  expect(readReminderPayload(reminderPayload(reminder()))).toEqual(reminderPayload(reminder()));
});

it('refuses a payload it cannot understand rather than throwing', () => {
  // A notification can outlive the app version that scheduled it. Opening a
  // stale one must do nothing, not crash.
  for (const junk of [null, undefined, 'text', 42, {}, { reminderId: 'only-one-field' }]) {
    expect(readReminderPayload(junk)).toBeNull();
  }
});
