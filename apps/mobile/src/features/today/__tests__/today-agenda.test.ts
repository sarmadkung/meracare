import type { Senior, SeniorSummary, TodayItem } from '@meracare/contracts';

import { attention, buildAgenda, peopleInDay } from '../today-agenda';

/**
 * Today is one timeline that spans every circle, narrowed by a person filter.
 *
 * Everything here is decided without React so it can be read and tested on its
 * own: what a row says, which row is the current one, what has slipped, and who
 * appears in the filter strip.
 */

const NOW = new Date('2026-09-08T09:40:00.000Z');

function item(overrides: Partial<TodayItem> = {}): TodayItem {
  return {
    kind: 'dose',
    id: 'dose-1',
    seniorId: 'senior-1',
    seniorName: 'Amina Bibi',
    timezone: 'Asia/Karachi',
    title: 'Metformin',
    detail: '500 mg',
    scheduledFor: '2026-09-08T09:00:00.000Z',
    status: 'pending',
    assignedToMe: false,
    ...overrides,
  };
}

function agenda(items: TodayItem[], namePeople = true) {
  return buildAgenda(items, { now: NOW, namePeople });
}

// --- what a row says --------------------------------------------------------

/**
 * A daughter in London checking on her mother in Karachi has to see the time
 * her mother will experience. 09:00 UTC is 14:00 there.
 */
it("reads each time in the senior's own zone", () => {
  expect(agenda([item()])[0]?.time).toBe('14:00');
});

/** Unfiltered, the list spans circles, so a row without a name says nothing. */
it('names whose care a row is while the day spans people', () => {
  expect(agenda([item()])[0]?.detail).toContain('Amina Bibi');
});

/**
 * Once one person is chosen their name moves to the heading, and the space it
 * was taking on every row goes back to what the row is actually for.
 */
it('gives the row back to the dosage once one person is chosen', () => {
  const entry = agenda([item()], false)[0];

  expect(entry?.detail).toContain('500 mg');
  expect(entry?.detail).not.toContain('Amina Bibi');
});

it('says how soon the current thing is due', () => {
  expect(agenda([item({ scheduledFor: '2026-09-08T10:00:00.000Z' })])[0]?.detail).toContain(
    'in 20 min',
  );
});

/** Ids collide across domains, so a row key has to include the kind. */
it('keys a row by kind and id together', () => {
  expect(agenda([item({ kind: 'task', id: 'x' })])[0]?.id).toBe('task:x');
  expect(agenda([item({ kind: 'dose', id: 'x' })])[0]?.id).toBe('dose:x');
});

it('sends a task to the task screen and a dose to the medication list', () => {
  expect(agenda([item({ kind: 'task', id: 't1' })])[0]?.href).toEqual({
    pathname: '/tasks/[taskId]',
    params: { taskId: 't1' },
  });

  // A dose has no screen of its own; the senior's medication list is where its
  // history and schedule live.
  expect(agenda([item({ kind: 'dose', seniorId: 's1' })])[0]?.href).toEqual({
    pathname: '/seniors/[seniorId]/medications',
    params: { seniorId: 's1' },
  });
});

// --- the time gutter --------------------------------------------------------

/** The gutter prints a time once and lets the rail carry the rest. */
it('prints a shared time only once', () => {
  const entries = agenda([item({ id: 'a' }), item({ id: 'b', title: 'Aspirin' })]);

  expect(entries.map((entry) => entry.showTime)).toEqual([true, false]);
});

/**
 * Two people in different zones can both be due at "14:00" and mean different
 * moments. Collapsing those would claim a coincidence that is not one.
 */
it('prints the time again when the same reading belongs to another zone', () => {
  const entries = agenda([
    item({ id: 'a' }),
    item({
      id: 'b',
      seniorId: 'senior-2',
      seniorName: 'Yusuf Khan',
      timezone: 'Europe/London',
      scheduledFor: '2026-09-08T13:00:00.000Z',
    }),
  ]);

  expect(entries.map((entry) => entry.time)).toEqual(['14:00', '14:00']);
  expect(entries.map((entry) => entry.showTime)).toEqual([true, true]);
});

// --- where the day has got to ------------------------------------------------

it('marks the first thing still to do as the current one', () => {
  const entries = agenda([
    item({ id: 'a', status: 'taken' }),
    item({ id: 'b', scheduledFor: '2026-09-08T10:00:00.000Z' }),
    item({ id: 'c', scheduledFor: '2026-09-08T15:00:00.000Z' }),
  ]);

  expect(entries.map((entry) => entry.tone)).toEqual(['done', 'now', 'upcoming']);
});

/**
 * A missed dose is not where you are in the day — it is behind you. The current
 * item is the next thing that can still go right.
 */
it('does not treat something already missed as the current one', () => {
  const entries = agenda([
    item({ id: 'a', status: 'missed' }),
    item({ id: 'b', scheduledFor: '2026-09-08T10:00:00.000Z' }),
  ]);

  expect(entries.map((entry) => entry.tone)).toEqual(['attention', 'now']);
});

/**
 * Seeing that the morning went well is most of what a family member opens the
 * app for, so finished work stays on the line rather than disappearing.
 */
it('keeps finished work on the line, said in words', () => {
  const entries = agenda([
    item({ id: 'a', status: 'taken' }),
    item({ id: 'b', kind: 'task', status: 'completed' }),
    item({ id: 'c', status: 'skipped' }),
  ]);

  expect(entries.map((entry) => entry.status)).toEqual(['Taken', 'Done', 'Skipped']);
});

it('says nothing about something that is simply not due yet', () => {
  expect(agenda([item({ scheduledFor: '2026-09-08T15:00:00.000Z' })])[0]?.status).toBeNull();
});

it('calls a missed dose missed and an overdue task overdue', () => {
  const entries = agenda([
    item({ id: 'a', status: 'missed' }),
    item({ id: 'b', kind: 'task', status: 'overdue' }),
  ]);

  expect(entries.map((entry) => entry.status)).toEqual(['Missed', 'Overdue']);
  expect(entries.map((entry) => entry.tone)).toEqual(['attention', 'attention']);
});

// --- what can be settled from here ------------------------------------------

it('offers to settle a dose or a task from the day itself', () => {
  expect(agenda([item({ kind: 'dose', id: 'd1' })])[0]?.action).toEqual({
    kind: 'dose',
    id: 'd1',
    seniorId: 'senior-1',
  });
  expect(agenda([item({ kind: 'task', id: 't1' })])[0]?.action).toEqual({
    kind: 'task',
    id: 't1',
    seniorId: 'senior-1',
  });
});

/**
 * Nobody knows from a phone whether someone attended an appointment, so there
 * is nothing here to record — docs/03 gives an appointment no derived status.
 */
it('offers nothing to settle on an appointment', () => {
  expect(agenda([item({ kind: 'appointment' })])[0]?.action).toBeNull();
});

it('stops offering an action once the outcome is recorded', () => {
  expect(agenda([item({ status: 'taken' })])[0]?.action).toBeNull();
});

// --- what has slipped --------------------------------------------------------

it('names the one thing that slipped', () => {
  const found = attention(agenda([item({ status: 'missed' }), item({ id: 'b' })]));

  expect(found?.title).toBe('Metformin was missed');
  expect(found?.detail).toContain('14:00');
});

it('says an overdue task is overdue rather than missed', () => {
  const found = attention(agenda([item({ kind: 'task', title: 'Bathe', status: 'overdue' })]));

  expect(found?.title).toBe('Bathe is overdue');
});

it('counts them once more than one has slipped', () => {
  const found = attention(
    agenda([
      item({ id: 'a', title: 'Amlodipine', status: 'missed' }),
      item({ id: 'b', kind: 'task', title: 'Blood pressure check', status: 'overdue' }),
    ]),
  );

  expect(found?.title).toBe('2 need attention');
  expect(found?.detail).toBe('Amlodipine · Blood pressure check');
});

it('says nothing when the day is on track', () => {
  expect(attention(agenda([item(), item({ id: 'b', status: 'taken' })]))).toBeNull();
});

// --- the person strip ---------------------------------------------------------

function senior(overrides: Partial<Senior> = {}): Senior {
  return {
    id: 'senior-1',
    displayName: 'Amina Bibi',
    dateOfBirth: null,
    photoUrl: null,
    phone: null,
    address: null,
    emergencyContact: null,
    timezone: 'Asia/Karachi',
    isSelf: false,
    role: 'family_member',
    permissions: [],
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
    ...overrides,
  };
}

function summary(seniorId: string, needsAttention: number): SeniorSummary {
  return { seniorId, medications: null, tasks: null, needsAttention };
}

/**
 * Your own care is a fixed place in the strip; everyone else is ordered by how
 * much is going wrong, so a professional with six clients sees whoever is
 * slipping without scrolling.
 */
it('puts your own care first, then whoever needs attention most', () => {
  const people = peopleInDay(
    [
      senior({ id: 'a', displayName: 'Amina' }),
      senior({ id: 'me', displayName: 'You', isSelf: true }),
      senior({ id: 'y', displayName: 'Yusuf' }),
    ],
    [summary('a', 0), summary('y', 3)],
  );

  expect(people.map((person) => person.seniorId)).toEqual(['me', 'y', 'a']);
});

it('marks the people who have something missed or overdue', () => {
  const people = peopleInDay([senior({ id: 'a' }), senior({ id: 'y' })], [summary('y', 2)]);

  expect(people.find((person) => person.seniorId === 'y')?.needsAttention).toBe(true);
  expect(people.find((person) => person.seniorId === 'a')?.needsAttention).toBe(false);
});

/**
 * A summary the reader has no permission to see is absent rather than zero, and
 * an absent summary is not evidence that nothing is wrong — it is no evidence
 * at all, so the dot stays off rather than guessing.
 */
it('does not invent a warning for a person whose summary has not arrived', () => {
  expect(peopleInDay([senior({ id: 'a' })], [])[0]?.needsAttention).toBe(false);
});
