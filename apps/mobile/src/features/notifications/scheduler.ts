import type { Reminder, ReminderPlan } from '@genxcare/contracts';
import { reminderBody, reminderPayload, reminderTitle } from '@genxcare/contracts';
import * as Notifications from 'expo-notifications';

import { reconcile, type ScheduledNotification } from './reconcile';

/**
 * The only place that talks to the operating system's notification scheduler.
 *
 * Reminders are scheduled by the OS, not by GenXcare: there is no timer, no
 * interval, and no background process kept alive to notice that a dose is due.
 * The device is told once, when the app is open, and the OS delivers whether or
 * not the app is running (docs/08-notifications-and-background.md,
 * plans/phase8.md §20).
 */

/**
 * How a reminder behaves when it fires while GenXcare is open.
 *
 * It is still shown. A caregiver reading last week's activity when the eight
 * o'clock dose comes due needs telling, and a notification suppressed because
 * the app happened to be in the foreground is a reminder that silently did not
 * happen (docs/08-notifications-and-background.md).
 */
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldPlaySound: true,
    shouldSetBadge: false,
    shouldShowBanner: true,
    shouldShowList: true,
  }),
});

/** Where the reminder's firing instant is kept, so reconciliation can read it. */
const FIRE_AT = 'fireAt';

/**
 * Schedules one reminder, using the server's identifier as the OS identifier.
 *
 * That shared identifier is the whole duplicate-prevention mechanism: asking
 * the OS to schedule an identifier it already holds replaces it rather than
 * adding a second (plans/phase8.md §§25, 26).
 */
async function schedule(reminder: Reminder): Promise<void> {
  await Notifications.scheduleNotificationAsync({
    identifier: reminder.id,
    content: {
      title: reminderTitle(reminder),
      body: reminderBody(reminder),
      // Identifiers only. Whatever the screen shows is fetched afterwards under
      // the user's own authorization (plans/phase8.md §16).
      data: { ...reminderPayload(reminder), [FIRE_AT]: reminder.fireAt },
    },
    trigger: {
      type: Notifications.SchedulableTriggerInputTypes.DATE,
      date: new Date(reminder.fireAt),
    },
  });
}

/** Reads what the OS currently has pending for us. */
async function pending(): Promise<ScheduledNotification[]> {
  const scheduled = await Notifications.getAllScheduledNotificationsAsync();

  return scheduled.map((notification) => {
    // Read the instant back from the payload rather than from the trigger. The
    // trigger's shape differs between iOS and Android and has changed across
    // Expo versions; our own field has not.
    const raw = notification.content.data?.[FIRE_AT];
    const fireAtMs = typeof raw === 'string' ? Date.parse(raw) : Number.NaN;

    return {
      identifier: notification.identifier,
      fireAtMs: Number.isFinite(fireAtMs) ? fireAtMs : 0,
    };
  });
}

/** What one reconciliation did, for logging and for tests. */
export interface SyncResult {
  scheduled: number;
  cancelled: number;
}

/**
 * Brings the device's scheduled notifications in line with the plan.
 *
 * Safe to run as often as it likes to be run — after a launch, a refresh, a
 * preference change, or a failed attempt. It is a comparison of two lists, and
 * running it twice in a row does nothing the second time.
 */
export async function syncReminders(plan: ReminderPlan): Promise<SyncResult> {
  const work = reconcile(plan, await pending(), Date.now());

  // Cancel first. If the app is killed midway, having cancelled a stale
  // reminder and not yet scheduled a new one is the safer half to have done:
  // a missing reminder is a gap, a stale one is misinformation.
  for (const identifier of work.cancel) {
    await Notifications.cancelScheduledNotificationAsync(identifier);
  }
  for (const reminder of work.schedule) {
    await schedule(reminder);
  }

  return { scheduled: work.schedule.length, cancelled: work.cancel.length };
}

/**
 * Cancels every reminder this app has scheduled.
 *
 * Used on sign-out. Reminders describe a specific person's care, and leaving
 * them on a phone that has been handed to somebody else would be a disclosure
 * (plans/phase8.md §§17, 23).
 */
export async function clearReminders(): Promise<void> {
  await Notifications.cancelAllScheduledNotificationsAsync();
}
