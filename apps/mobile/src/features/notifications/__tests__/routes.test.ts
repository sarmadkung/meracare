import type { ReminderPayload } from '@meracare/contracts';
import { readPushPayload } from '@meracare/contracts';

import { notificationDestination, reminderDestination } from '../routes';

/**
 * Every notification has somewhere to go. A tap that lands on the wrong screen
 * — or nowhere — makes the reminder useless at exactly the moment it mattered.
 */

function payload(overrides: Partial<ReminderPayload> = {}): ReminderPayload {
  return {
    reminderId: 'reminder-1',
    type: 'MEDICATION_REMINDER',
    seniorId: 'senior-1',
    entityType: 'medication_dose',
    entityId: 'dose-1',
    ...overrides,
  };
}

it('opens today’s medication for a dose', () => {
  expect(reminderDestination(payload())).toEqual({
    pathname: '/seniors/[seniorId]/medications',
    params: { seniorId: 'senior-1' },
  });
});

it('opens the task for a task reminder', () => {
  const destination = reminderDestination(
    payload({ type: 'TASK_REMINDER', entityType: 'task_instance', entityId: 'task-9' }),
  );

  expect(destination).toEqual({
    pathname: '/tasks/[taskId]',
    params: { taskId: 'task-9' },
  });
});

it('opens the appointment for an appointment reminder', () => {
  const destination = reminderDestination(
    payload({
      type: 'APPOINTMENT_REMINDER',
      entityType: 'appointment',
      entityId: 'appointment-3',
    }),
  );

  expect(destination).toEqual({
    pathname: '/appointments/[appointmentId]',
    params: { appointmentId: 'appointment-3' },
  });
});

/**
 * Phase 11 adds a second payload shape — the one the server pushes — and one
 * new destination for it. Both shapes route through the same function, so these
 * check the addition rather than repeating the three cases above.
 */

it('opens the activity timeline for a care-activity notification', () => {
  expect(
    notificationDestination({
      entityType: 'care_event',
      entityId: 'event-4',
      seniorId: 'senior-1',
    }),
  ).toEqual({
    pathname: '/seniors/[seniorId]/activity',
    params: { seniorId: 'senior-1' },
  });
});

it('routes a pushed payload exactly as a scheduled reminder', () => {
  const pushed = readPushPayload({
    notificationId: 'notification-1',
    type: 'MEDICATION_REMINDER',
    seniorId: 'senior-1',
    entityType: 'medication_dose',
    entityId: 'dose-1',
  });

  expect(pushed).not.toBeNull();
  expect(notificationDestination(pushed!)).toEqual(reminderDestination(payload()));
});

it('routes an overdue task to the task itself', () => {
  expect(
    notificationDestination({
      entityType: 'task_instance',
      entityId: 'task-9',
      seniorId: 'senior-1',
    }),
  ).toEqual({ pathname: '/tasks/[taskId]', params: { taskId: 'task-9' } });
});

describe('readPushPayload', () => {
  const valid = {
    notificationId: 'notification-1',
    type: 'CARE_ACTIVITY',
    seniorId: 'senior-1',
    entityType: 'care_event',
    entityId: 'event-1',
  };

  it('reads a complete payload', () => {
    expect(readPushPayload(valid)).toEqual(valid);
  });

  it.each([
    ['null', null],
    ['a string', 'nonsense'],
    ['a missing field', { ...valid, entityId: undefined }],
    ['an empty field', { ...valid, seniorId: '' }],
    ['an unknown type', { ...valid, type: 'SOMETHING_ELSE' }],
    ['an unknown entity', { ...valid, entityType: 'invoice' }],
  ])('returns null for %s rather than throwing', (_label, data) => {
    // A notification can outlive the app version that sent it. Opening a stale
    // one must do nothing, not crash the app.
    expect(readPushPayload(data)).toBeNull();
  });
});
