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
 * Native uses the keychain alongside the session. Web uses localStorage through
 * the platform-specific adapter because the identifier is not a credential and
 * must survive a tab closing. Auth tokens use the narrower sessionStorage on
 * web and never share this path.
 */
export async function deviceId(): Promise<string> {
  if (cached !== null) return cached;

  const existing = await secureStorage.getItem(DEVICE_ID_KEY);
  if (existing !== null && existing !== '') {
    cached = existing;
    return existing;
  }

  const created = newDeviceId();
  await secureStorage.setItem(DEVICE_ID_KEY, created);
  cached = created;
  return created;
}

/**
 * Mints an identifier for this installation.
 *
 * Hermes ships no WebCrypto, so `globalThis.crypto` is undefined on the device
 * and reaching for `randomUUID` there throws — which happens inside
 * `describeDevice`, so registration fails on every launch and sign-out, which
 * deactivates this device first, can never complete.
 *
 * This names one installation of the app; it is not a credential and nothing is
 * authorised by holding it, so a random string is enough. The same reasoning
 * gave the offline queue its operation ids (src/features/tasks/use-tasks.ts).
 */
function newDeviceId(): string {
  if (typeof globalThis.crypto?.randomUUID === 'function') {
    return globalThis.crypto.randomUUID();
  }

  const random = () => Math.random().toString(36).slice(2, 12);
  return `${Date.now().toString(36)}-${random()}-${random()}`;
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
