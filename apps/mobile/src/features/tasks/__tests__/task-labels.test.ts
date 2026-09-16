import type { CareTask, TaskStatus } from '@genxcare/contracts';
import {
  RECURRENCE_PRESETS,
  TASK_STATUSES,
  isOpen,
  isOverdue,
  recurrenceLabel,
  statusLabel,
  statusTone,
  taskDateLabel,
  taskTimeLabel,
} from '@genxcare/contracts';

/**
 * Nothing internal may reach a screen: not the rule the server stores, not a
 * status identifier, and not a time in the reader's timezone when the task
 * belongs to somebody else's day (plans/phase4.md §§14, 21, 34).
 */

function task(overrides: Partial<CareTask> = {}): CareTask {
  return {
    id: 'task-1',
    templateId: null,
    seniorId: 'senior-1',
    title: 'Morning walk',
    description: null,
    assignedUserId: null,
    scheduledFor: '2026-08-17T04:00:00Z',
    status: 'pending',
    recurring: false,
    completedAt: null,
    completedBy: null,
    skippedAt: null,
    skippedBy: null,
    notes: null,
    createdAt: '2026-08-16T09:00:00Z',
    updatedAt: '2026-08-16T09:00:00Z',
    ...overrides,
  };
}

describe('recurrenceLabel', () => {
  it('describes the everyday shapes the way somebody would say them', () => {
    expect(recurrenceLabel({ frequency: 'daily', weekdays: [] })).toBe('Every day');
    expect(
      recurrenceLabel({
        frequency: 'weekly',
        weekdays: ['monday', 'tuesday', 'wednesday', 'thursday', 'friday'],
      }),
    ).toBe('Every weekday');
    expect(recurrenceLabel({ frequency: 'weekly', weekdays: ['saturday', 'sunday'] })).toBe(
      'Every weekend',
    );
    expect(recurrenceLabel({ frequency: 'weekly', weekdays: ['monday'] })).toBe('Every Monday');
  });

  it('lists several days the way a person would', () => {
    expect(
      recurrenceLabel({ frequency: 'weekly', weekdays: ['monday', 'wednesday', 'friday'] }),
    ).toBe('Every Monday, Wednesday and Friday');
  });

  // The order the days arrive in must not change the wording.
  it('reads days in week order however they were chosen', () => {
    expect(
      recurrenceLabel({ frequency: 'weekly', weekdays: ['friday', 'monday', 'wednesday'] }),
    ).toBe('Every Monday, Wednesday and Friday');
  });

  it('treats all seven days as every day', () => {
    expect(
      recurrenceLabel({
        frequency: 'weekly',
        weekdays: ['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday'],
      }),
    ).toBe('Every day');
  });

  // The stored rule is an implementation detail of the server.
  it('never exposes the underlying rule', () => {
    for (const preset of RECURRENCE_PRESETS) {
      if (preset.recurrence === null) continue;
      const label = recurrenceLabel(preset.recurrence);

      expect(label).not.toContain('FREQ');
      expect(label).not.toContain('BYDAY');
      expect(label).not.toContain('=');
    }
  });
});

describe('statusLabel', () => {
  it('describes every status in plain words', () => {
    for (const status of TASK_STATUSES) {
      const label = statusLabel(status);

      expect(label.length).toBeGreaterThan(0);
      // The identifier itself must not be what the user reads.
      expect(label).not.toBe(status);
    }
  });

  it('says what an older adult would expect', () => {
    expect(statusLabel('pending')).toBe('To do');
    expect(statusLabel('overdue')).toBe('Overdue');
    expect(statusLabel('completed')).toBe('Done');
    expect(statusLabel('skipped')).toBe('Skipped');
  });
});

describe('statusTone', () => {
  // Colour reinforces the label; it never carries the meaning alone, so every
  // status must have wording as well as a tone (docs/18).
  it('gives every status a tone and a label', () => {
    for (const status of TASK_STATUSES) {
      expect(statusTone(status)).toBeDefined();
      expect(statusLabel(status)).toBeTruthy();
    }
  });

  it('marks an overdue task as needing attention', () => {
    expect(statusTone('overdue')).toBe('attention');
    expect(statusTone('completed')).toBe('positive');
  });
});

describe('isOpen', () => {
  it('counts pending and overdue tasks as still needing doing', () => {
    expect(isOpen(task({ status: 'pending' }))).toBe(true);
    expect(isOpen(task({ status: 'overdue' }))).toBe(true);
  });

  it('counts settled tasks as done with', () => {
    for (const status of ['completed', 'skipped', 'cancelled'] as TaskStatus[]) {
      expect(isOpen(task({ status }))).toBe(false);
    }
  });

  it('recognises an overdue task', () => {
    expect(isOverdue(task({ status: 'overdue' }))).toBe(true);
    expect(isOverdue(task({ status: 'pending' }))).toBe(false);
  });
});

describe('taskTimeLabel', () => {
  // 04:00 UTC is 09:00 in Karachi. A daughter in London must see her mother's
  // morning, not her own.
  it('renders the time in the senior s own timezone', () => {
    expect(taskTimeLabel(task(), 'Asia/Karachi')).toBe('09:00');
    expect(taskTimeLabel(task(), 'Europe/London')).toBe('05:00');
    expect(taskTimeLabel(task(), 'UTC')).toBe('04:00');
  });

  it('falls back rather than failing on an unknown timezone', () => {
    expect(() => taskTimeLabel(task(), 'Mars/Olympus_Mons')).not.toThrow();
  });
});

describe('taskDateLabel', () => {
  it('names the day in the senior s timezone', () => {
    // 22:00 UTC on the 17th is already the 18th in Karachi.
    const late = task({ scheduledFor: '2026-08-17T22:00:00Z' });

    expect(taskDateLabel(late, 'Asia/Karachi')).toContain('18');
    expect(taskDateLabel(late, 'Europe/London')).toContain('17');
  });
});
