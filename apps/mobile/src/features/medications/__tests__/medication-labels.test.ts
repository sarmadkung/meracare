import type { MedicationDose, MedicationSchedule, MedicationStatus } from '@genxcare/contracts';
import {
  doseDateLabel,
  doseStatusLabel,
  doseStatusTone,
  doseTimeLabel,
  formLabel,
  isDoseMissed,
  isDoseOpen,
  nextDoseLabel,
  scheduleLabel,
  schedulesLabel,
} from '@genxcare/contracts';

/**
 * What a person actually reads.
 *
 * Nothing internal reaches a screen: not the RRULE the server stores, not a
 * status identifier, not a UTC instant (plans/phase5.md §31).
 */

function schedule(overrides: Partial<MedicationSchedule> = {}): MedicationSchedule {
  return {
    id: 'schedule-1',
    medicationId: 'med-1',
    recurrence: { frequency: 'daily', weekdays: [] },
    scheduledTime: '08:00',
    active: true,
    createdAt: '2026-08-16T09:00:00Z',
    updatedAt: '2026-08-16T09:00:00Z',
    ...overrides,
  };
}

describe('doseStatusLabel', () => {
  it('says what happened in words, never an identifier', () => {
    const wording: Record<MedicationStatus, string> = {
      pending: 'To take',
      taken: 'Taken',
      skipped: 'Skipped',
      missed: 'Missed',
    };

    for (const [status, expected] of Object.entries(wording)) {
      expect(doseStatusLabel(status as MedicationStatus)).toBe(expected);
    }
  });

  // Status must be legible without relying on colour, so the words carry the
  // meaning and the tone is only an addition.
  it('pairs every status with a tone', () => {
    expect(doseStatusTone('missed')).toBe('attention');
    expect(doseStatusTone('taken')).toBe('positive');
    expect(doseStatusTone('pending')).toBe('neutral');
    expect(doseStatusTone('skipped')).toBe('muted');
  });
});

describe('scheduleLabel', () => {
  it('reads as somebody would say it', () => {
    expect(scheduleLabel(schedule())).toBe('Every day at 08:00');
    expect(
      scheduleLabel(
        schedule({
          recurrence: { frequency: 'weekly', weekdays: ['monday', 'wednesday', 'friday'] },
          scheduledTime: '09:00',
        }),
      ),
    ).toBe('Every Monday, Wednesday and Friday at 09:00');
  });

  it('never shows the rule the server stores', () => {
    expect(scheduleLabel(schedule())).not.toContain('FREQ');
  });
});

describe('schedulesLabel', () => {
  // The common case: one rule at two times, which reads far better as one
  // sentence than as two.
  it('joins several times under one rule', () => {
    expect(
      schedulesLabel([schedule(), schedule({ id: 'schedule-2', scheduledTime: '20:00' })]),
    ).toBe('Every day at 08:00 and 20:00');
  });

  it('keeps different rules apart', () => {
    const label = schedulesLabel([
      schedule(),
      schedule({
        id: 'schedule-2',
        recurrence: { frequency: 'weekly', weekdays: ['monday'] },
        scheduledTime: '20:00',
      }),
    ]);

    expect(label).toContain('Every day at 08:00');
    expect(label).toContain('Every Monday at 20:00');
  });

  // A stopped time of day is not what the medicine is taken at any more.
  it('ignores times that have been stopped', () => {
    expect(
      schedulesLabel([
        schedule(),
        schedule({ id: 'schedule-2', scheduledTime: '20:00', active: false }),
      ]),
    ).toBe('Every day at 08:00');
  });

  it('says so plainly when no time has been set', () => {
    expect(schedulesLabel([])).toBe('No times set yet');
  });
});

describe('doseTimeLabel', () => {
  // A daughter in London looking at her mother in Karachi must see the time her
  // mother will experience.
  it('reads the time in the senior‘s timezone', () => {
    const dose = { scheduledFor: '2026-08-17T03:00:00Z' } as Pick<MedicationDose, 'scheduledFor'>;

    expect(doseTimeLabel(dose, 'Asia/Karachi')).toBe('08:00');
    expect(doseTimeLabel(dose, 'Europe/London')).toBe('04:00');
  });

  it('shows the date in the same timezone', () => {
    const dose = { scheduledFor: '2026-08-17T19:00:00Z' } as Pick<MedicationDose, 'scheduledFor'>;

    // Midnight has already passed in Karachi while it is still the 17th in
    // London — the two readers are on different days.
    expect(doseDateLabel(dose, 'Asia/Karachi')).toContain('18 August');
    expect(doseDateLabel(dose, 'Europe/London')).toContain('17 August');
  });

  // A screen that crashed on an unrecognised zone would be worse than one
  // showing a slightly wrong hour.
  it('survives a timezone it does not recognise', () => {
    const dose = { scheduledFor: '2026-08-17T03:00:00Z' } as Pick<MedicationDose, 'scheduledFor'>;

    expect(() => doseTimeLabel(dose, 'Mars/Olympus_Mons')).not.toThrow();
  });
});

describe('nextDoseLabel', () => {
  it('says when the next dose is', () => {
    expect(nextDoseLabel('2026-08-17T03:00:00Z', 'Asia/Karachi')).toContain('08:00');
  });

  it('says plainly when there is none', () => {
    expect(nextDoseLabel(null, 'Asia/Karachi')).toBe('Nothing scheduled');
  });
});

describe('formLabel', () => {
  it('names the form the way a label would', () => {
    expect(formLabel('tablet')).toBe('Tablet');
    expect(formLabel('inhaler')).toBe('Inhaler');
  });
});

describe('isDoseOpen', () => {
  it('counts a missed dose as still needing taking', () => {
    // A missed dose is pending underneath, so it can be taken late — which is
    // what people do.
    expect(isDoseOpen({ status: 'missed' })).toBe(true);
    expect(isDoseOpen({ status: 'pending' })).toBe(true);
    expect(isDoseOpen({ status: 'taken' })).toBe(false);
    expect(isDoseOpen({ status: 'skipped' })).toBe(false);
  });

  it('reports a missed dose', () => {
    expect(isDoseMissed({ status: 'missed' })).toBe(true);
    expect(isDoseMissed({ status: 'pending' })).toBe(false);
  });
});
