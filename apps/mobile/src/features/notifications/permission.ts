import * as Notifications from 'expo-notifications';

/**
 * The operating system's half of notification permission.
 *
 * Two separate things decide whether a reminder appears: what GenxCare has been
 * asked to send, and what the OS allows. A user can have every category
 * switched on in settings and still see nothing because they declined the
 * system prompt a year ago, and an app that conflates the two will insist
 * reminders are working while the phone stays silent (plans/phase8.md §6).
 */

/** What the OS currently allows. */
export type PermissionState =
  /** Not asked yet — the prompt is still available. */
  | 'undetermined'
  /** Full permission. */
  | 'granted'
  /**
   * Quiet delivery: iOS's provisional authorization, granted without a prompt.
   * Notifications arrive, but silently and in the notification centre only.
   */
  | 'provisional'
  /** Refused, or restricted by a device policy. The prompt will not reappear. */
  | 'denied';

/** Reads the current state without prompting. */
export async function notificationPermission(): Promise<PermissionState> {
  return interpret(await Notifications.getPermissionsAsync());
}

/**
 * Asks for permission, if asking is still possible.
 *
 * Once refused, the OS will not show the prompt again — asking merely returns
 * the refusal. The settings screen therefore points the user at the system
 * settings instead of offering a button that does nothing.
 */
export async function requestNotificationPermission(): Promise<PermissionState> {
  const current = await Notifications.getPermissionsAsync();
  if (!current.canAskAgain) return interpret(current);

  return interpret(await Notifications.requestPermissionsAsync());
}

/** Whether the OS will deliver anything at all in this state. */
export function permissionAllowsDelivery(state: PermissionState): boolean {
  return state === 'granted' || state === 'provisional';
}

function interpret(response: Notifications.NotificationPermissionsStatus): PermissionState {
  if (response.status === 'undetermined') return 'undetermined';

  // iOS reports provisional authorization through a separate field; the status
  // is 'granted' either way, and the difference is whether reminders make a
  // sound. Treating them as identical would be a small lie on the settings
  // screen, which is exactly where the user goes to find out why it is quiet.
  if (response.ios?.status === Notifications.IosAuthorizationStatus.PROVISIONAL) {
    return 'provisional';
  }
  if (response.granted) return 'granted';

  return 'denied';
}
