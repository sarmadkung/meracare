import type { CareEvent, CareEventType } from '@genxcare/contracts';
import {
  CARE_EVENT_TYPES,
  careEventCategory,
  careEventDescription,
  careEventTimeLabel,
  careEventTone,
  dayHeading,
  groupByDay,
} from '@genxcare/contracts';

/**
 * The wording somebody actually reads. Two things matter most: no raw
 * identifier can reach a screen, and a day boundary is the senior's, not UTC's
 * (plans/phase7.md §§14, 16).
 */

function event(overrides: Partial<CareEvent> = {}): CareEvent {
  return {
    id: 'event-1',
    seniorId: 'senior-1',
    type: 'TASK_COMPLETED',
    actorUserId: 'user-1',
    entityType: 'task',
    entityId: 'task-1',
    metadata: { taskTitle: 'Morning walk' },
    // 10:42 in Karachi.
    occurredAt: '2026-08-18T05:42:00Z',
    ...overrides,
  };
}

describe('event wording', () => {
  it('reads as a sentence about a person, never as an identifier', () => {
    expect(careEventDescription(event(), 'Sarah')).toBe('Sarah marked Morning walk as done');
    expect(
      careEventDescription(
        event({ type: 'MEDICATION_TAKEN', metadata: { medicationName: 'Metformin' } }),
        'Ahmed',
      ),
    ).toBe('Ahmed recorded Metformin as taken');
    expect(
      careEventDescription(
        event({ type: 'APPOINTMENT_CANCELLED', metadata: { appointmentTitle: 'Cardiology' } }),
        'Sarah',
      ),
    ).toBe('Sarah cancelled Cardiology');
  });

  // The guarantee behind plans/phase7.md §14: whatever the server sends, no
  // screen can end up showing TASK_COMPLETED.
  it('has readable wording for every event type the API can send', () => {
    for (const type of CARE_EVENT_TYPES) {
      const description = careEventDescription(event({ type: type as CareEventType }), 'Sarah');

      expect(description).toBeTruthy();
      expect(description).not.toContain('_');
      expect(description).not.toMatch(/[A-Z]{4,}/);
      expect(careEventCategory(type as CareEventType)).toBeTruthy();
      expect(careEventTone(type as CareEventType)).toBeTruthy();
    }
  });

  // Somebody who has left the circle can no longer be looked up, and their
  // past entries must still read sensibly.
  it('copes with an actor it cannot name', () => {
    expect(careEventDescription(event(), 'Somebody')).toBe('Somebody marked Morning walk as done');
  });

  // Metadata is best-effort: an event whose subject was not recorded should
  // still produce a sentence rather than "undefined".
  it('copes with missing metadata', () => {
    const bare = careEventDescription(event({ metadata: {} }), 'Sarah');
    expect(bare).toBe('Sarah marked a task as done');
    expect(bare).not.toContain('undefined');
  });

  it('keeps most of the feed quiet', () => {
    // A feed where every row is coloured is a feed where nothing stands out.
    const tones = CARE_EVENT_TYPES.map((type) => careEventTone(type as CareEventType));
    expect(tones.filter((tone) => tone === 'neutral').length).toBeGreaterThan(0);
  });

  it('shows the time in the senior‘s timezone', () => {
    expect(careEventTimeLabel(event(), 'Asia/Karachi')).toBe('10:42');
    expect(careEventTimeLabel(event(), 'Europe/London')).toBe('06:42');
  });
});

describe('day grouping', () => {
  const timezone = 'Asia/Karachi';

  it('names today and yesterday', () => {
    const now = new Date('2026-08-18T12:00:00Z');
    expect(dayHeading('2026-08-18T05:42:00Z', timezone, now)).toBe('Today');
    expect(dayHeading('2026-08-17T05:42:00Z', timezone, now)).toBe('Yesterday');
  });

  it('falls back to the date for anything older', () => {
    const now = new Date('2026-08-18T12:00:00Z');
    expect(dayHeading('2026-08-15T05:42:00Z', timezone, now)).toContain('15 August');
  });

  // The reason grouping cannot use the UTC date: 00:30 in Karachi is 19:30 the
  // previous day in UTC, and filing it under yesterday would be wrong for
  // everybody looking at this senior's care.
  it('groups an event just after midnight under the senior‘s day', () => {
    const now = new Date('2026-08-18T12:00:00Z');
    // 2026-08-18T00:30 in Karachi.
    const justAfterMidnight = '2026-08-17T19:30:00Z';

    expect(dayHeading(justAfterMidnight, timezone, now)).toBe('Today');
    // The same instant read in London is still the 17th, and is grouped there.
    expect(dayHeading(justAfterMidnight, 'Europe/London', now)).toBe('Yesterday');
  });

  it('groups a newest-first list into newest-first days', () => {
    const now = new Date('2026-08-18T12:00:00Z');
    const events = [
      event({ id: 'a', occurredAt: '2026-08-18T05:42:00Z' }),
      event({ id: 'b', occurredAt: '2026-08-18T04:00:00Z' }),
      event({ id: 'c', occurredAt: '2026-08-17T13:15:00Z' }),
    ];

    const days = groupByDay(events, timezone, now);

    expect(days.map((day) => day.heading)).toEqual(['Today', 'Yesterday']);
    expect(days[0]?.events.map((entry) => entry.id)).toEqual(['a', 'b']);
    expect(days[1]?.events.map((entry) => entry.id)).toEqual(['c']);
  });

  it('gives each day a stable key for a list', () => {
    const days = groupByDay([event()], timezone, new Date('2026-08-18T12:00:00Z'));
    expect(days[0]?.key).toBe('2026-08-18');
  });

  it('groups an empty timeline into no days', () => {
    expect(groupByDay([], timezone)).toEqual([]);
  });
});
