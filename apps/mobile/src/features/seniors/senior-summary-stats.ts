import type { SeniorSummary } from '@meracare/contracts';

import type { SummaryStat } from '@/components/ui';

/**
 * Turns one senior's day into the chips their card shows.
 *
 * A domain the reader cannot see contributes nothing — not a zero. Zero doses
 * is a fact about the senior; absence is a fact about the reader, and showing
 * "0/0" to somebody without medications.view would tell them something untrue.
 *
 * Tasks are shown as what is left rather than what is done, because the number
 * a caregiver acts on is the one still waiting.
 *
 * Kept apart from the query that feeds it so it can be read and tested without
 * dragging in an API client.
 */
export function summaryStats(summary: SeniorSummary | undefined): SummaryStat[] {
  if (summary === undefined) return [];

  const stats: SummaryStat[] = [];

  if (summary.medications !== null) {
    stats.push({
      value: `${summary.medications.done}/${summary.medications.total}`,
      label: 'Meds',
      tone: 'brand',
    });
  }

  if (summary.tasks !== null) {
    stats.push({
      value: String(summary.tasks.total - summary.tasks.done),
      label: 'Tasks',
      tone: 'neutral',
    });
  }

  // Only when there is something to act on. A permanent "0 Due" chip is noise
  // that trains people to stop reading the row.
  if (summary.needsAttention > 0) {
    stats.push({ value: String(summary.needsAttention), label: 'Due', tone: 'warning' });
  }

  return stats;
}
