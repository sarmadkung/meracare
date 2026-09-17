import Constants from 'expo-constants';
import * as Notifications from 'expo-notifications';

import { secureStorage } from '@/lib/secure-storage';

import { describeDevice, deviceId, resetDeviceIdCache } from '../device';

/**
 * The installation identifier is what makes registration an update rather than
 * an accumulation. If it changed between launches, the server would collect a
 * new device row every time the app started, and a later push phase would send
 * one notification per launch the app had ever had (plans/phase8.md §§7, 25).
 */

jest.mock('@/lib/secure-storage', () => {
  const store = new Map<string, string>();
  return {
    secureStorage: {
      getItem: jest.fn(async (key: string) => store.get(key) ?? null),
      setItem: jest.fn(async (key: string, value: string) => {
        store.set(key, value);
      }),
      removeItem: jest.fn(async (key: string) => {
        store.delete(key);
      }),
    },
  };
});

jest.mock('expo-notifications', () => ({
  getPermissionsAsync: jest.fn(),
  requestPermissionsAsync: jest.fn(),
  getExpoPushTokenAsync: jest.fn(),
  IosAuthorizationStatus: { PROVISIONAL: 3 },
}));

const getPermissions = Notifications.getPermissionsAsync as jest.Mock;
const getPushToken = Notifications.getExpoPushTokenAsync as jest.Mock;

beforeEach(() => {
  resetDeviceIdCache();
  getPermissions.mockReset().mockResolvedValue({
    status: 'granted',
    granted: true,
    canAskAgain: false,
  });
  getPushToken.mockReset().mockResolvedValue({ data: 'ExponentPushToken[abc]' });
});

it('creates an identifier once and keeps it', async () => {
  const first = await deviceId();

  // A fresh process, as after the app is force-quit and reopened.
  resetDeviceIdCache();
  const second = await deviceId();

  expect(second).toBe(first);
  expect(secureStorage.setItem).toHaveBeenCalledTimes(1);
});

it('describes the installation for registration', async () => {
  const description = await describeDevice();

  expect(description.deviceId).toBe(await deviceId());
  expect(['ios', 'android', 'web']).toContain(description.platform);
  expect(description.appVersion).toBe(Constants.expoConfig?.version ?? '');
});

it('carries a push token once permission allows delivery', async () => {
  // Only meaningful with an EAS project id configured; without one there is no
  // token to fetch, which is the state GenxCare is actually in today.
  const projectId = Constants.expoConfig?.extra?.eas?.projectId;

  const description = await describeDevice();

  if (typeof projectId === 'string' && projectId !== '') {
    expect(description.pushToken).toBe('ExponentPushToken[abc]');
  } else {
    expect(description.pushToken).toBeUndefined();
    expect(getPushToken).not.toHaveBeenCalled();
  }
});

it('registers without a token when notifications are refused', async () => {
  // A device we may not push to is still a device we know about. Refusing to
  // register it would mean re-deriving that state on every launch.
  getPermissions.mockResolvedValue({ status: 'denied', granted: false, canAskAgain: false });

  const description = await describeDevice();

  expect(description.pushToken).toBeUndefined();
  expect(description.deviceId).not.toBe('');
  expect(getPushToken).not.toHaveBeenCalled();
});

it('registers without a token when the push service cannot be reached', async () => {
  // Push credentials are not configured, so this is the ordinary path rather
  // than an exotic one. It must not throw (plans/phase8.md §37).
  getPushToken.mockRejectedValue(new Error('no credentials'));

  await expect(describeDevice()).resolves.toMatchObject({ pushToken: undefined });
});

/**
 * Hermes has no WebCrypto. `globalThis.crypto` is undefined on the device, so
 * an identifier minted through it throws — and because that throw happens
 * inside `describeDevice`, registration fails on every launch and sign-out,
 * which deactivates this device first, can never complete.
 */
it('creates an identifier in a runtime without WebCrypto', async () => {
  await secureStorage.removeItem('genxcare.deviceId');
  resetDeviceIdCache();

  const descriptor = Object.getOwnPropertyDescriptor(globalThis, 'crypto');
  Object.defineProperty(globalThis, 'crypto', { value: undefined, configurable: true });

  try {
    const id = await deviceId();

    expect(typeof id).toBe('string');
    expect(id).not.toBe('');
  } finally {
    if (descriptor !== undefined) Object.defineProperty(globalThis, 'crypto', descriptor);
  }
});
