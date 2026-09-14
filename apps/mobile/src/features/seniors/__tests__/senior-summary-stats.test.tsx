import type { SeniorSummary } from '@genxcare/contracts';

import { summaryStats } from '../senior-summary-stats';

/**
 * Turns one senior's day into the stat chips their card shows.
 *
 * The rules that matter are about silence: a domain the reader cannot see says
 * nothing at all, and a count of zero is not the same as no permission.
 */

function summary(overrides: Partial<SeniorSummary> = {}): SeniorSummary {
  return {
    seniorId: 'senior-1',
    medications: { done: 3, total: 4 },
    tasks: { done: 2, total: 4 },
    needsAttention: 0,
    ...overrides,
  };
}

it('shows doses taken, tasks left, and what is late', () => {
  expect(summaryStats(summary({ needsAttention: 1 }))).toEqual([
    { value: '3/4', label: 'Meds', tone: 'brand' },
    { value: '2', label: 'Tasks', tone: 'neutral' },
    { value: '1', label: 'Due', tone: 'warning' },
  ]);
});

/** Nothing late is the normal case, and an empty chip would be noise. */
it('says nothing about lateness when nothing is late', () => {
  expect(summaryStats(summary()).some((stat) => stat.label === 'Due')).toBe(false);
});

/**
 * A caregiver without medications.view must not learn that the senior takes
 * medication at all — so the chip is absent, not zero.
 */
it('omits a domain the reader cannot see', () => {
  const stats = summaryStats(summary({ medications: null }));

  expect(stats.some((stat) => stat.label === 'Meds')).toBe(false);
  expect(stats.some((stat) => stat.label === 'Tasks')).toBe(true);
});

/** Nothing scheduled is worth saying: it means the day is genuinely clear. */
it('reports an empty day as zeroes', () => {
  expect(
    summaryStats(summary({ medications: { done: 0, total: 0 }, tasks: { done: 0, total: 0 } })),
  ).toEqual([
    { value: '0/0', label: 'Meds', tone: 'brand' },
    { value: '0', label: 'Tasks', tone: 'neutral' },
  ]);
});

/** A senior with no summary yet renders a plain card rather than a broken one. */
it('has nothing to show without a summary', () => {
  expect(summaryStats(undefined)).toEqual([]);
});
