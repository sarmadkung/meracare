import type { Reminder, ReminderPlan } from '@genxcare/contracts';
import * as Notifications from 'expo-notifications';

import { clearReminders, syncReminders } from '../scheduler';

/**
 * What the operating system is actually told. The reconciliation arithmetic is
 * covered in reconcile.test.ts; this covers the translation into scheduling
 * calls — in particular that the server's identifier is used as the OS
 * identifier, which is what makes a repeat schedule a replacement rather than a
 * duplicate (plans/phase8.md §§25, 26).
 */

jest.mock('expo-notifications', () => ({
  setNotificationHandler: jest.fn(),
  scheduleNotificationAsync: jest.fn().mockResolvedValue('id'),
  cancelScheduledNotificationAsync: jest.fn().mockResolvedValue(undefined),
  cancelAllScheduledNotificationsAsync: jest.fn().mockResolvedValue(undefined),
  getAllScheduledNotificationsAsync: jest.fn().mockResolvedValue([]),
  SchedulableTriggerInputTypes: { DATE: 'date' },
}));

const schedule = Notifications.scheduleNotificationAsync as jest.Mock;
const cancel = Notifications.cancelScheduledNotificationAsync as jest.Mock;
const cancelAll = Notifications.cancelAllScheduledNotificationsAsync as jest.Mock;
const pending = Notifications.getAllScheduledNotificationsAsync as jest.Mock;

function reminder(overrides: Partial<Reminder> = {}): Reminder {
  return {
    id: 'reminder-1',
    type: 'MEDICATION_REMINDER',
    seniorId: 'senior-1',
    seniorName: 'Amma',
    seniorTimezone: 'Asia/Karachi',
    entityType: 'medication_dose',
    entityId: 'dose-1',
    dueAt: '2099-08-19T03:00:00Z',
    fireAt: '2099-08-19T02:45:00Z',
    ...overrides,
  };
}

function plan(reminders: Reminder[]): ReminderPlan {
  return {
    reminders,
    generatedAt: '2026-08-19T06:00:00Z',
    horizonEndsAt: '2099-12-31T00:00:00Z',
  };
}

beforeEach(() => {
  schedule.mockClear();
  cancel.mockClear();
  cancelAll.mockClear();
  pending.mockReset().mockResolvedValue([]);
});

it('schedules a reminder under the server’s identifier', async () => {
  const result = await syncReminders(plan([reminder()]));

  expect(result).toEqual({ scheduled: 1, cancelled: 0 });
  expect(schedule).toHaveBeenCalledTimes(1);

  const request = schedule.mock.calls[0][0];
  expect(request.identifier).toBe('reminder-1');
  expect(request.trigger).toEqual({
    type: 'date',
    date: new Date('2099-08-19T02:45:00Z'),
  });
});

it('gives the notification privacy-conscious wording and an identifier-only payload', async () => {
  await syncReminders(plan([reminder()]));

  const { content } = schedule.mock.calls[0][0];

  expect(content.title).toBe('Medication reminder');
  expect(content.body).not.toContain('mg');
  expect(content.data).toMatchObject({
    reminderId: 'reminder-1',
    seniorId: 'senior-1',
    entityType: 'medication_dose',
    entityId: 'dose-1',
  });
});

it('schedules nothing on a second run', async () => {
  // The app reconciles after every launch. One reminder must not become two.
  const only = reminder();

  await syncReminders(plan([only]));

  pending.mockResolvedValue([
    {
      identifier: only.id,
      content: { data: { fireAt: only.fireAt } },
    },
  ]);

  const again = await syncReminders(plan([only]));

  expect(again).toEqual({ scheduled: 0, cancelled: 0 });
  expect(schedule).toHaveBeenCalledTimes(1);
});

it('cancels a reminder the plan no longer contains', async () => {
  pending.mockResolvedValue([
    { identifier: 'gone', content: { data: { fireAt: '2099-08-19T02:45:00Z' } } },
  ]);

  const result = await syncReminders(plan([]));

  expect(result).toEqual({ scheduled: 0, cancelled: 1 });
  expect(cancel).toHaveBeenCalledWith('gone');
});

it('cancels before it schedules', async () => {
  // Interrupted midway, having cancelled a stale reminder is safer than having
  // scheduled a new one alongside it.
  pending.mockResolvedValue([
    { identifier: 'stale', content: { data: { fireAt: '2099-08-19T02:45:00Z' } } },
  ]);

  const order: string[] = [];
  cancel.mockImplementation(async () => {
    order.push('cancel');
  });
  schedule.mockImplementation(async () => {
    order.push('schedule');
    return 'id';
  });

  await syncReminders(plan([reminder({ id: 'fresh' })]));

  expect(order).toEqual(['cancel', 'schedule']);
});

it('clears everything on sign-out', async () => {
  await clearReminders();

  expect(cancelAll).toHaveBeenCalled();
});
