import type { TodayItem } from '@meracare/contracts';

import { splitByAssignment, toRow } from '../today-rows';

function item(overrides: Partial<TodayItem> = {}): TodayItem {
  return {
    kind: 'dose',
    id: 'dose-1',
    seniorId: 'senior-1',
    seniorName: 'James',
    timezone: 'Asia/Karachi',
    title: 'Metformin',
    detail: '500 mg',
    scheduledFor: '2026-09-08T09:00:00.000Z',
    status: 'pending',
    assignedToMe: false,
    ...overrides,
  };
}

/**
 * A daughter in London checking on her mother in Karachi has to see the time
 * her mother will experience. 09:00 UTC is 14:00 there.
 */
it("reads the time in the senior's own zone", () => {
  expect(toRow(item()).subtitle).toContain('14:00');
});

/** This list spans circles, so a row without a name says nothing useful. */
it('names whose care each row is', () => {
  expect(toRow(item()).subtitle).toContain('James');
});

it('carries the dosage when there is one', () => {
  expect(toRow(item()).subtitle).toContain('500 mg');
});

it('says nothing extra when there is no detail', () => {
  expect(toRow(item({ detail: '' })).subtitle).toBe('14:00 · James');
});

/** Ids collide across domains, so the row key has to include the kind. */
it('keys a row by kind and id together', () => {
  expect(toRow(item({ kind: 'task', id: 'x' })).id).toBe('task:x');
  expect(toRow(item({ kind: 'dose', id: 'x' })).id).toBe('dose:x');
});

it('sends a task to the task screen and a dose to the medication list', () => {
  expect(toRow(item({ kind: 'task', id: 't1' })).href).toEqual({
    pathname: '/tasks/[taskId]',
    params: { taskId: 't1' },
  });

  // A dose has no screen of its own; the senior's medication list is where it
  // can actually be acted on.
  expect(toRow(item({ kind: 'dose', seniorId: 's1' })).href).toEqual({
    pathname: '/seniors/[seniorId]/medications',
    params: { seniorId: 's1' },
  });
});

/**
 * Assignment leads but does not filter. A family member caring alone assigns
 * nothing to themselves, and a Today built only from assignments would be
 * permanently empty for them — which is exactly how this screen shipped.
 */
it('leads with the reader’s own work without hiding the rest', () => {
  const { mine, rest } = splitByAssignment([
    item({ id: 'a', assignedToMe: true, title: 'Mine' }),
    item({ id: 'b', assignedToMe: false, title: 'Unassigned' }),
  ]);

  expect(mine.map((row) => row.title)).toEqual(['Mine']);
  expect(rest.map((row) => row.title)).toEqual(['Unassigned']);
});
