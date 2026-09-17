import * as Notifications from 'expo-notifications';

import {
  notificationPermission,
  permissionAllowsDelivery,
  requestNotificationPermission,
} from '../permission';

/**
 * The operating system's permission is separate from GenxCare's preferences,
 * and confusing the two is how an app ends up insisting reminders are on while
 * the phone stays silent (plans/phase8.md §6).
 */

jest.mock('expo-notifications', () => ({
  getPermissionsAsync: jest.fn(),
  requestPermissionsAsync: jest.fn(),
  IosAuthorizationStatus: { PROVISIONAL: 3 },
}));

const getPermissions = Notifications.getPermissionsAsync as jest.Mock;
const requestPermissions = Notifications.requestPermissionsAsync as jest.Mock;

beforeEach(() => {
  getPermissions.mockReset();
  requestPermissions.mockReset();
});

it('reports permission that has not been asked for', async () => {
  getPermissions.mockResolvedValue({ status: 'undetermined', granted: false, canAskAgain: true });

  await expect(notificationPermission()).resolves.toBe('undetermined');
});

it('reports granted permission', async () => {
  getPermissions.mockResolvedValue({ status: 'granted', granted: true, canAskAgain: false });

  await expect(notificationPermission()).resolves.toBe('granted');
});

it('distinguishes iOS provisional authorization from full permission', async () => {
  // Provisional means reminders arrive silently. Calling that "granted" would
  // leave the settings screen unable to explain why the phone is quiet.
  getPermissions.mockResolvedValue({
    status: 'granted',
    granted: true,
    canAskAgain: false,
    ios: { status: 3 },
  });

  await expect(notificationPermission()).resolves.toBe('provisional');
});

it('reports refusal', async () => {
  getPermissions.mockResolvedValue({ status: 'denied', granted: false, canAskAgain: false });

  await expect(notificationPermission()).resolves.toBe('denied');
});

it('prompts when the prompt is still available', async () => {
  getPermissions.mockResolvedValue({ status: 'undetermined', granted: false, canAskAgain: true });
  requestPermissions.mockResolvedValue({ status: 'granted', granted: true, canAskAgain: false });

  await expect(requestNotificationPermission()).resolves.toBe('granted');
  expect(requestPermissions).toHaveBeenCalled();
});

it('does not prompt again once refused', async () => {
  // The OS will not show it a second time, so asking would be a button that
  // visibly does nothing. The settings screen sends the user to Settings.
  getPermissions.mockResolvedValue({ status: 'denied', granted: false, canAskAgain: false });

  await expect(requestNotificationPermission()).resolves.toBe('denied');
  expect(requestPermissions).not.toHaveBeenCalled();
});

it('knows which states deliver anything', () => {
  expect(permissionAllowsDelivery('granted')).toBe(true);
  expect(permissionAllowsDelivery('provisional')).toBe(true);
  expect(permissionAllowsDelivery('denied')).toBe(false);
  expect(permissionAllowsDelivery('undetermined')).toBe(false);
});
