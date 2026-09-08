import type { TodayItem } from '@meracare/contracts';
import { timeInTimezone } from '@meracare/contracts';

import type { IconName } from '@/components/ui';

/**
 * One row on Today, ready to render.
 *
 * Kept apart from the query that feeds it so it can be read and tested without
 * dragging in an API client.
 */
export interface TodayRow {
  id: string;
  icon: IconName;
  title: string;
  /** Time, whose care it is, and anything else worth a glance. */
  subtitle: string;
  href: { pathname: string; params: Record<string, string> };
}

const icons: Record<TodayItem['kind'], IconName> = {
  task: 'task',
  dose: 'pill',
  appointment: 'calendar',
};

const destinations: Record<TodayItem['kind'], { pathname: string; param: string }> = {
  task: { pathname: '/tasks/[taskId]', param: 'taskId' },
  dose: { pathname: '/seniors/[seniorId]/medications', param: 'seniorId' },
  appointment: { pathname: '/appointments/[appointmentId]', param: 'appointmentId' },
};

/**
 * Describes one item for the list.
 *
 * The time is rendered in the senior's own zone, never the reader's: a daughter
 * in London checking on her mother in Karachi needs the time her mother will
 * experience. The senior's name is on every row because this list spans
 * circles, and "Metformin, 2:00 pm" is meaningless without knowing whose.
 */
export function toRow(item: TodayItem): TodayRow {
  const when = timeInTimezone(item.scheduledFor, item.timezone);
  const parts = [when, item.seniorName];
  if (item.detail !== '') parts.push(item.detail);

  const destination = destinations[item.kind];

  return {
    id: `${item.kind}:${item.id}`,
    icon: icons[item.kind],
    title: item.title,
    subtitle: parts.join(' · '),
    href: {
      pathname: destination.pathname,
      params: { [destination.param]: destination.param === 'seniorId' ? item.seniorId : item.id },
    },
  };
}

/**
 * Splits today into the reader's own work and everything else.
 *
 * Assignment leads because a professional working a round needs their own list
 * first. It is not a filter, though: a family member caring alone assigns
 * nothing to themselves, and a Today built only from assignments would be
 * permanently empty for them.
 */
export function splitByAssignment(items: TodayItem[]): { mine: TodayRow[]; rest: TodayRow[] } {
  return {
    mine: items.filter((item) => item.assignedToMe).map(toRow),
    rest: items.filter((item) => !item.assignedToMe).map(toRow),
  };
}
