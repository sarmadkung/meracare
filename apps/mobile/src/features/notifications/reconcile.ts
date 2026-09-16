import type { Reminder, ReminderPlan } from '@genxcare/contracts';

/**
 * Working out what the operating system should be told.
 *
 * Kept as a pure function over two lists, separately from the code that talks
 * to expo-notifications, because this is where every duplicate-notification and
 * stale-notification bug would live and it is worth being able to test it
 * without a device (plans/phase8.md §§22, 26).
 */

/** What the OS currently has pending, reduced to what matters here. */
export interface ScheduledNotification {
  identifier: string;
  /** When it will fire, as an epoch millisecond value. */
  fireAtMs: number;
}

/** The two lists of work a reconciliation produces. */
export interface Reconciliation {
  /** Identifiers to cancel: scheduled, but no longer in the plan. */
  cancel: string[];
  /** Reminders to schedule: in the plan, but not scheduled. */
  schedule: Reminder[];
}

/**
 * Compares the plan against what the device has pending.
 *
 * Both directions matter, and for different reasons. Scheduling what is missing
 * is the obvious half. Cancelling what is no longer planned is the half that
 * keeps a cancelled appointment from still buzzing, and a revoked caregiver's
 * phone from continuing to announce a family's care (plans/phase8.md §§22, 23).
 *
 * Reminder identifiers come from the server and are a pure function of the
 * reminder's meaning, so "already scheduled" is an exact comparison rather than
 * a guess. That is what makes running this after every launch, every refresh,
 * and every retry safe (plans/phase8.md §25).
 */
export function reconcile(
  plan: ReminderPlan,
  scheduled: ScheduledNotification[],
  nowMs: number,
): Reconciliation {
  const planned = new Map(plan.reminders.map((reminder) => [reminder.id, reminder]));
  const horizonMs = Date.parse(plan.horizonEndsAt);

  const cancel: string[] = [];
  const alreadyScheduled = new Set<string>();

  for (const notification of scheduled) {
    if (planned.has(notification.identifier)) {
      alreadyScheduled.add(notification.identifier);
      continue;
    }

    // Anything due beyond the plan's horizon is not absent from the plan — it
    // is outside it. Cancelling those would silently shorten every reminder to
    // the horizon, and the user would never learn why the app stopped telling
    // them about next month's appointment.
    if (Number.isFinite(horizonMs) && notification.fireAtMs > horizonMs) continue;

    cancel.push(notification.identifier);
  }

  const schedule = plan.reminders.filter(
    // A reminder whose moment passed between the server building the plan and
    // the device acting on it cannot be scheduled, and firing it late would be
    // worse than not firing it.
    (reminder) => !alreadyScheduled.has(reminder.id) && Date.parse(reminder.fireAt) > nowMs,
  );

  return { cancel, schedule };
}
