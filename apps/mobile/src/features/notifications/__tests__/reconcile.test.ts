import type { Reminder, ReminderPlan } from '@genxcare/contracts';

import { reconcile, type ScheduledNotification } from '../reconcile';

/**
 * Reconciliation is where every duplicate-notification and stale-notification
 * bug would live, so it is tested as a pure function over two lists rather than
 * through a device.
 */

const NOW = Date.parse('2026-08-19T06:00:00Z');

function reminder(overrides: Partial<Reminder> = {}): Reminder {
  return {
    id: 'reminder-1',
    type: 'MEDICATION_REMINDER',
    seniorId: 'senior-1',
    seniorName: 'Amma',
    seniorTimezone: 'Asia/Karachi',
    entityType: 'medication_dose',
    entityId: 'dose-1',
    dueAt: '2026-08-19T08:00:00Z',
    fireAt: '2026-08-19T07:45:00Z',
    ...overrides,
  };
}

function plan(reminders: Reminder[], horizonEndsAt = '2026-08-26T06:00:00Z'): ReminderPlan {
  return { reminders, generatedAt: '2026-08-19T06:00:00Z', horizonEndsAt };
}

function scheduled(identifier: string, fireAt: string): ScheduledNotification {
  return { identifier, fireAtMs: Date.parse(fireAt) };
}

it('schedules everything when the device has nothing', () => {
  const work = reconcile(plan([reminder()]), [], NOW);

  expect(work.schedule).toHaveLength(1);
  expect(work.cancel).toEqual([]);
});

it('schedules nothing the second time', () => {
  // The property the whole design rests on: the app reconciles after every
  // launch and every refresh, and a reminder is scheduled exactly once.
  const only = reminder();
  const work = reconcile(plan([only]), [scheduled(only.id, only.fireAt)], NOW);

  expect(work.schedule).toEqual([]);
  expect(work.cancel).toEqual([]);
});

it('cancels a reminder that has left the plan', () => {
  // What a cancelled appointment or a stopped medicine looks like from here.
  const work = reconcile(plan([]), [scheduled('reminder-1', '2026-08-19T07:45:00Z')], NOW);

  expect(work.cancel).toEqual(['reminder-1']);
  expect(work.schedule).toEqual([]);
});

it('replaces a reminder whose time has moved', () => {
  // The server gives a moved reminder a new identifier, so the old one is
  // cancelled and the new one scheduled — rather than a stale alert surviving.
  const moved = reminder({ id: 'reminder-2', fireAt: '2026-08-19T08:45:00Z' });

  const work = reconcile(plan([moved]), [scheduled('reminder-1', '2026-08-19T07:45:00Z')], NOW);

  expect(work.cancel).toEqual(['reminder-1']);
  expect(work.schedule.map((item) => item.id)).toEqual(['reminder-2']);
});

it('cancels everything when the plan is empty', () => {
  // A revoked caregiver's plan. Their phone must stop announcing the family's
  // care, and it must not need the server to say so item by item.
  const work = reconcile(
    plan([]),
    [
      scheduled('a', '2026-08-19T07:45:00Z'),
      scheduled('b', '2026-08-20T07:45:00Z'),
      scheduled('c', '2026-08-21T07:45:00Z'),
    ],
    NOW,
  );

  expect(work.cancel).toEqual(['a', 'b', 'c']);
});

it('leaves alone a notification scheduled beyond the plan horizon', () => {
  // Absent from the plan because it is past its edge, not because it was
  // cancelled. Cancelling it would silently shorten every reminder to the
  // horizon and nobody would know why.
  const work = reconcile(
    plan([], '2026-08-26T06:00:00Z'),
    [scheduled('far-future', '2026-09-30T07:45:00Z')],
    NOW,
  );

  expect(work.cancel).toEqual([]);
});

it('does not schedule a reminder whose moment has already passed', () => {
  // The plan was built a few minutes ago; this one fired in between.
  const stale = reminder({ fireAt: '2026-08-19T05:45:00Z' });
  const work = reconcile(plan([stale]), [], NOW);

  expect(work.schedule).toEqual([]);
});

it('is stable when run repeatedly against its own result', () => {
  // Running twice in a row must be a no-op the second time, whatever the state.
  const reminders = [
    reminder({ id: 'a', fireAt: '2026-08-19T07:45:00Z' }),
    reminder({ id: 'b', fireAt: '2026-08-20T07:45:00Z' }),
  ];

  const first = reconcile(plan(reminders), [scheduled('gone', '2026-08-19T06:30:00Z')], NOW);
  expect(first.cancel).toEqual(['gone']);
  expect(first.schedule).toHaveLength(2);

  const after = first.schedule.map((item) => scheduled(item.id, item.fireAt));
  const second = reconcile(plan(reminders), after, NOW);

  expect(second.cancel).toEqual([]);
  expect(second.schedule).toEqual([]);
});

it('treats an unreadable pending notification as cancellable', () => {
  // A notification scheduled by an older version of the app, with no firing
  // instant in its payload. It is not in the plan and cannot be shown to be
  // beyond the horizon, so it goes — leaving it would be leaving a reminder
  // this app can no longer reason about at all.
  const work = reconcile(plan([]), [{ identifier: 'legacy', fireAtMs: 0 }], NOW);

  expect(work.cancel).toEqual(['legacy']);
});
