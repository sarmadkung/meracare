import Constants from 'expo-constants';
import * as Notifications from 'expo-notifications';
import { Platform } from 'react-native';

import { secureStorage } from '@/lib/secure-storage';

import { notificationPermission, permissionAllowsDelivery } from './permission';

import type { DevicePlatform, DeviceRegistration } from '@genxcare/contracts';

/**
 * This installation's identity, and its push token when it has one.
 *
 * The identifier is generated once and kept, so the server sees one device
 * rather than a new one after every launch. That is what makes registration an
 * update instead of an accumulation (plans/phase8.md §§7, 25).
 */

const DEVICE_ID_KEY = 'genxcare.deviceId';

let cached: string | null = null;

/**
 * Returns this installation's identifier, creating it on first use.
 *
 * Stored in the keychain alongside the session rather than in plain storage —
 * not because it is a secret, but because it is the key the server uses to
 * recognise this phone, and a value that survives a reinstall of the app but
 * not a change of device is exactly what the keychain gives.
 */
export async function deviceId(): Promise<string> {
  if (cached !== null) return cached;

  const existing = await secureStorage.getItem(DEVICE_ID_KEY);
  if (existing !== null && existing !== '') {
    cached = existing;
    return existing;
  }

  const created = globalThis.crypto.randomUUID();
  await secureStorage.setItem(DEVICE_ID_KEY, created);
  cached = created;
  return created;
}

/** Forgets the cached identifier. Used by tests; the stored value is untouched. */
export function resetDeviceIdCache(): void {
  cached = null;
}

/** The platform name the API recognises. */
function platform(): DevicePlatform {
  switch (Platform.OS) {
    case 'ios':
      return 'ios';
    case 'android':
      return 'android';
    default:
      return 'web';
  }
}

/**
 * Fetches an Expo push token, or nothing.
 *
 * Nothing is the normal answer today: a token requires notification permission
 * and an EAS project id, and GenXcare has no push credentials configured yet.
 * The failure is caught rather than propagated because push is not what makes
 * reminders work — those are scheduled on the device — and an app that refused
 * to start because it could not obtain a push token would be broken for a
 * feature it is not yet using (plans/phase8.md §§9, 37).
 */
async function pushToken(): Promise<string | undefined> {
  if (!permissionAllowsDelivery(await notificationPermission())) return undefined;

  const projectId =
    Constants.expoConfig?.extra?.eas?.projectId ?? Constants.easConfig?.projectId ?? undefined;
  if (typeof projectId !== 'string' || projectId === '') return undefined;

  try {
    const token = await Notifications.getExpoPushTokenAsync({ projectId });
    return token.data;
  } catch {
    return undefined;
  }
}

/** Describes this installation for the registration endpoint. */
export async function describeDevice(): Promise<DeviceRegistration> {
  return {
    deviceId: await deviceId(),
    platform: platform(),
    pushToken: await pushToken(),
    appVersion: Constants.expoConfig?.version ?? '',
  };
}
