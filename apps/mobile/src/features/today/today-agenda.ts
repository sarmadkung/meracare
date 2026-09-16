import type { Senior, SeniorSummary, TodayItem, TodayItemKind } from '@genxcare/contracts';
import { timeInTimezone } from '@genxcare/contracts';

import type { AgendaTone, IconName } from '@/components/ui';

/**
 * Today, as one timeline that can be narrowed to one person.
 *
 * Kept apart from the query that feeds it and from the screen that draws it, so
 * the decisions worth arguing about — which row is the current one, what counts
 * as slipping, whose name a row carries — can be read and tested on their own.
 */

/** Enough to record an outcome without leaving the screen. */
export interface AgendaAction {
  kind: 'dose' | 'task';
  id: string;
  seniorId: string;
}

/** One thing happening today, ready to draw on the timeline. */
export interface AgendaEntry {
  id: string;
  kind: TodayItemKind;
  icon: IconName;
  title: string;
  /** Whose care it is while the day spans people; the dosage once it does not. */
  detail: string;
  /** In the senior's own zone, never the reader's. */
  time: string;
  /** False when the row above is the same moment — the gutter prints it once. */
  showTime: boolean;
  tone: AgendaTone;
  /** The word on the status pill, or null when the state needs no saying. */
  status: string | null;
  assignedToMe: boolean;
  seniorId: string;
  href: { pathname: string; params: Record<string, string> };
  /** Absent on an appointment, and on anything already settled. */
  action: AgendaAction | null;
}

/** One face in the filter strip. */
export interface FilterPerson {
  seniorId: string;
  name: string;
  isSelf: boolean;
  needsAttention: boolean;
  /** Their own zone, which is the one the heading names a city from. */
  timezone: string;
}

const icons: Record<TodayItemKind, IconName> = {
  task: 'task',
  dose: 'pill',
  appointment: 'calendar',
};

const destinations: Record<TodayItemKind, { pathname: string; param: string }> = {
  task: { pathname: '/tasks/[taskId]', param: 'taskId' },
  dose: { pathname: '/seniors/[seniorId]/medications', param: 'seniorId' },
  appointment: { pathname: '/appointments/[appointmentId]', param: 'appointmentId' },
};

/**
 * The word shown on a row's pill.
 *
 * Meaning never rests on colour alone (docs/18), so every state that matters
 * has a word. `pending` and `scheduled` are absent deliberately: "not due yet"
 * is what a row with no pill already means.
 */
const statusWords: Record<string, string> = {
  taken: 'Taken',
  completed: 'Done',
  skipped: 'Skipped',
  cancelled: 'Cancelled',
  missed: 'Missed',
  overdue: 'Overdue',
};

/** Somebody has already decided the outcome. */
function settled(status: string): boolean {
  return (
    status === 'taken' || status === 'completed' || status === 'skipped' || status === 'cancelled'
  );
}

/** The hour has passed and nothing was recorded. */
function slipped(status: string): boolean {
  return status === 'missed' || status === 'overdue';
}

export interface AgendaOptions {
  now: Date;
  /**
   * True while the day spans everyone. Each row then carries the senior's name,
   * because "Metformin at 2pm" says nothing without knowing whose 2pm.
   */
  namePeople: boolean;
}

/**
 * Builds the timeline.
 *
 * Expects the items in time order, which is how `GET /v1/today` returns them:
 * a caregiver works through their day in order, not one client at a time.
 */
export function buildAgenda(items: TodayItem[], { now, namePeople }: AgendaOptions): AgendaEntry[] {
  // The current row is the next thing that can still go right. Something
  // already missed is behind you, not where you are.
  const currentIndex = items.findIndex((item) => !settled(item.status) && !slipped(item.status));

  let previousMoment: string | null = null;

  return items.map((item, index) => {
    const time = timeInTimezone(item.scheduledFor, item.timezone);
    // Two zones can read the same on the clock and mean different moments, so
    // the zone is part of what makes two rows the same time.
    const moment = `${item.timezone}|${time}`;
    const showTime = moment !== previousMoment;
    previousMoment = moment;

    const tone: AgendaTone = settled(item.status)
      ? 'done'
      : slipped(item.status)
        ? 'attention'
        : index === currentIndex
          ? 'now'
          : 'upcoming';

    const destination = destinations[item.kind];

    return {
      id: `${item.kind}:${item.id}`,
      kind: item.kind,
      icon: icons[item.kind],
      title: titleOf(item, namePeople),
      detail: describe(item, { namePeople, soon: tone === 'now' ? until(item, now) : null }),
      time,
      showTime,
      tone,
      status: statusWords[item.status] ?? null,
      assignedToMe: item.assignedToMe,
      seniorId: item.seniorId,
      href: {
        pathname: destination.pathname,
        params: {
          [destination.param]: destination.param === 'seniorId' ? item.seniorId : item.id,
        },
      },
      action: settleable(item),
    };
  });
}

/**
 * What the row is called.
 *
 * Under Everyone the line beneath is taken by the senior's name, so a dose has
 * to carry its dosage in the title or it is not on the screen at all. A task's
 * detail is not a dosage: "Morning walk 20 minutes" is a sentence, not a title.
 */
function titleOf(item: TodayItem, namePeople: boolean): string {
  if (!namePeople || item.kind !== 'dose' || item.detail === '') return item.title;

  return `${item.title} ${item.detail}`;
}

/** The line under a row's title. */
function describe(
  item: TodayItem,
  { namePeople, soon }: { namePeople: boolean; soon: string | null },
): string {
  const base = namePeople ? item.seniorName : item.detail;
  return [base, soon].filter((part) => part !== null && part !== '').join(' · ');
}

/**
 * How long until the current thing is due.
 *
 * Only ever shown on the current row: a relative time on every row would be a
 * screenful of numbers counting down, and the absolute time is already in the
 * gutter beside it.
 */
function until(item: TodayItem, now: Date): string {
  const minutes = Math.round((new Date(item.scheduledFor).getTime() - now.getTime()) / 60_000);

  if (minutes <= 0) return 'due now';
  if (minutes < 60) return `in ${minutes} min`;

  const hours = Math.round(minutes / 60);
  return hours === 1 ? 'in 1 hour' : `in ${hours} hours`;
}

/**
 * What can be recorded from here.
 *
 * An appointment has nothing to record: nobody knows from a phone whether the
 * person went, which is why docs/03 gives it no derived status. Anything
 * already settled has nothing left to say either.
 */
function settleable(item: TodayItem): AgendaAction | null {
  if (item.kind === 'appointment' || settled(item.status)) return null;

  return { kind: item.kind, id: item.id, seniorId: item.seniorId };
}

/** Where a row lives, which is also where its slip can be settled. */
export type Destination = { pathname: string; params: Record<string, string> };

export interface Attention {
  title: string;
  detail: string;
  /** The one thing that slipped, or null once several have. */
  href: Destination | null;
}

/** What the banner at the top of the day says, or null when nothing has slipped. */
export function attention(entries: AgendaEntry[]): Attention | null {
  const slipping = entries.filter((entry) => entry.tone === 'attention');

  if (slipping.length === 0) return null;

  // One thing can be named and gone to. Several cannot be named without the
  // banner becoming the list it sits above, or pointed at without choosing one.
  const [only] = slipping;
  if (slipping.length === 1 && only !== undefined) {
    return {
      title: `${only.title} ${only.status === 'Missed' ? 'was missed' : 'is overdue'}`,
      detail: `Due ${only.time} · record it or mark it skipped`,
      href: only.href,
    };
  }

  return {
    title: `${slipping.length} need attention`,
    detail: slipping
      .slice(0, 2)
      .map((entry) => entry.title)
      .join(' · '),
    href: null,
  };
}

/** How the chosen person's day is going, said under their name. */
export interface DaySummary {
  /** "4 left", or "All done". */
  left: string;
  /** "2 need attention", or null while the day is on track. */
  attention: string | null;
  /** The city whose clock the times are on, or null for a zone naming none. */
  place: string | null;
}

/**
 * The line under the heading.
 *
 * It exists mostly for the city. Times are drawn in the senior's zone, so a
 * daughter in London reading "14:00" needs to be told whose 14:00 it is — and
 * once there is a line there, what is left and what has slipped belong on it.
 */
export function daySummary(entries: AgendaEntry[], timezone: string): DaySummary {
  const left = entries.filter((entry) => entry.tone !== 'done').length;
  const slipping = entries.filter((entry) => entry.tone === 'attention').length;

  return {
    // "0 left" is a number to decode; "All done" is the thing it means.
    left: left === 0 ? 'All done' : `${left} left`,
    attention:
      slipping === 0 ? null : slipping === 1 ? '1 needs attention' : `${slipping} need attention`,
    place: cityIn(timezone),
  };
}

/**
 * The city in an IANA zone name: `Asia/Karachi` is Karachi.
 *
 * A zone with no region in it names no city, and inventing one would be a lie
 * about whose clock a reader is looking at.
 */
function cityIn(timezone: string): string | null {
  const parts = timezone.split('/');
  const city = parts[parts.length - 1];

  if (parts.length < 2 || city === undefined || city === '') return null;

  return city.replace(/_/g, ' ');
}

/**
 * The faces in the filter strip.
 *
 * Your own care keeps a fixed place; everyone else is ordered by how much is
 * going wrong, so a professional with six clients sees whoever is slipping
 * without scrolling the strip.
 */
export function peopleInDay(seniors: Senior[], summaries: SeniorSummary[]): FilterPerson[] {
  const bySenior = new Map(summaries.map((entry) => [entry.seniorId, entry]));

  return seniors
    .map((senior) => ({
      seniorId: senior.id,
      name: senior.displayName,
      isSelf: senior.isSelf,
      timezone: senior.timezone,
      // An absent summary is not evidence that nothing is wrong — it is no
      // evidence at all, so the dot stays off rather than guessing.
      needsAttention: (bySenior.get(senior.id)?.needsAttention ?? 0) > 0,
    }))
    .sort((a, b) => {
      if (a.isSelf !== b.isSelf) return a.isSelf ? -1 : 1;

      const attentionA = bySenior.get(a.seniorId)?.needsAttention ?? 0;
      const attentionB = bySenior.get(b.seniorId)?.needsAttention ?? 0;
      if (attentionA !== attentionB) return attentionB - attentionA;

      return a.name.localeCompare(b.name);
    });
}
