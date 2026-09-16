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

/** Shared with the Expo push request built by the API. */
export const MEDICATION_ACTION_CATEGORY = 'medication_actions';
export const MEDICATION_TAKEN_ACTION = 'medication_taken';
export const MEDICATION_SKIP_ACTION = 'medication_skip';
export const MEDICATION_SNOOZE_ACTION = 'medication_snooze';

const SNOOZE_PREFIX = 'genxcare-snooze-';
const SNOOZE_SECONDS = 10 * 60;

/** Registers the buttons the OS displays on medication notifications. */
export async function registerMedicationNotificationActions(): Promise<void> {
  await Notifications.setNotificationCategoryAsync(MEDICATION_ACTION_CATEGORY, [
    {
      identifier: MEDICATION_TAKEN_ACTION,
      buttonTitle: 'Taken',
      options: { opensAppToForeground: true, isAuthenticationRequired: true },
    },
    {
      identifier: MEDICATION_SKIP_ACTION,
      buttonTitle: 'Skip',
      options: { opensAppToForeground: true, isAuthenticationRequired: true },
    },
    {
      identifier: MEDICATION_SNOOZE_ACTION,
      buttonTitle: 'Remind in 10 min',
      options: { opensAppToForeground: true },
    },
  ]);
}

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
      categoryIdentifier:
        reminder.type === 'MEDICATION_REMINDER' ? MEDICATION_ACTION_CATEGORY : undefined,
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

  return scheduled
    .filter((notification) => !notification.identifier.startsWith(SNOOZE_PREFIX))
    .map((notification) => {
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

/** Schedules a privacy-preserving one-off follow-up from an existing notification. */
export async function snoozeMedicationNotification(
  notification: Notifications.Notification,
): Promise<void> {
  const fireAt = new Date(Date.now() + SNOOZE_SECONDS * 1000).toISOString();
  const content = notification.request.content;

  await Notifications.scheduleNotificationAsync({
    identifier: `${SNOOZE_PREFIX}${Date.now()}`,
    content: {
      title: content.title ?? 'Medication reminder',
      body: content.body ?? 'A dose needs your attention.',
      data: { ...content.data, [FIRE_AT]: fireAt },
      categoryIdentifier: MEDICATION_ACTION_CATEGORY,
    },
    trigger: {
      type: Notifications.SchedulableTriggerInputTypes.TIME_INTERVAL,
      seconds: SNOOZE_SECONDS,
      repeats: false,
    },
  });
}

/** Cancels this device's follow-ups once the dose has been dealt with. */
export async function cancelSnoozedMedicationNotifications(doseId: string): Promise<void> {
  const scheduled = await Notifications.getAllScheduledNotificationsAsync();
  for (const notification of scheduled) {
    if (
      notification.identifier.startsWith(SNOOZE_PREFIX) &&
      notification.content.data?.entityId === doseId
    ) {
      await Notifications.cancelScheduledNotificationAsync(notification.identifier);
    }
  }
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
