import { readPushPayload, readReminderPayload } from '@meracare/contracts';
import * as Notifications from 'expo-notifications';
import { router, type Href } from 'expo-router';
import { useEffect, useRef } from 'react';
import { AppState } from 'react-native';

import { useUIStore } from '@/stores/ui-store';

import { notificationDestination, reminderDestination } from './routes';
import { clearReminders, syncReminders } from './scheduler';
import { notificationPermission, permissionAllowsDelivery } from './permission';
import { notificationKeys, useRegisterDevice, useReminderPlan } from './use-notifications';

import { useQueryClient } from '@tanstack/react-query';

/**
 * Keeps the device's scheduled reminders in step with the server's plan.
 *
 * Mounted once, near the root. Everything it does is a reconciliation rather
 * than an instruction, so running it more often than necessary is harmless and
 * running it too rarely is the only real failure mode — which is why it runs on
 * sign-in, on every plan change, and whenever the app comes back to the
 * foreground (plans/phase8.md §§22, 25).
 */
export function useReminderSync(isSignedIn: boolean, isRestoring: boolean) {
  const queryClient = useQueryClient();
  const plan = useReminderPlan(isSignedIn);
  const register = useRegisterDevice();

  // Held in a ref rather than state: nothing renders from it, and the mutation
  // object is recreated on every render.
  const registerRef = useRef(register.mutate);
  registerRef.current = register.mutate;

  // Announce this installation once per sign-in.
  useEffect(() => {
    if (!isSignedIn) return;
    registerRef.current();
  }, [isSignedIn]);

  // Whether the server can reach this installation.
  //
  // This is what decides who schedules the reminders, and it has to be decided
  // by exactly one of them. Phase 11 gave the server a push path; leaving the
  // device's local scheduling switched on as well would show every reminder
  // twice — once from the OS, once from the push (plans/phase11.md §35).
  //
  // The server wins when it can: a push reflects the care as it is at the
  // moment it is sent, where a locally scheduled reminder reflects the plan as
  // it was when the app was last open. Local scheduling remains the fallback,
  // and today it is the only path that works at all, because MeraCare holds no
  // push credentials yet.
  const registrationSettled = register.isSuccess || register.isError;
  const serverCanPush = register.data?.pushTokenRegistered === true;

  // Apply each plan as it arrives.
  useEffect(() => {
    if (!isSignedIn || plan.data === undefined) return;
    // Until registration has answered, neither party knows who is responsible.
    // Scheduling now and cancelling a moment later would show a reminder and
    // then take it away.
    if (!registrationSettled) return;

    let cancelled = false;

    const current = plan.data;

    void (async () => {
      if (serverCanPush) {
        // The server is delivering these. Anything this device scheduled
        // earlier — before permission was granted, or before it had a token —
        // has to go, or the next dose is announced twice.
        await clearReminders().catch(() => {});
        return;
      }

      // No point scheduling anything the OS will not deliver. The reminders
      // stay in the plan; they are simply not scheduled until permission
      // exists, at which point this runs again (plans/phase8.md §6).
      if (!permissionAllowsDelivery(await notificationPermission())) return;
      if (cancelled) return;

      try {
        await syncReminders(current);
      } catch {
        // A device that refuses to schedule must not break the app. The care
        // itself is on the screens either way, and the next reconciliation will
        // try again (plans/phase8.md §37).
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [isSignedIn, plan.data, registrationSettled, serverCanPush]);

  // Refetch when the app is reopened. A phone that has been closed for two days
  // has a plan two days old, and the reminders at the far end of it were never
  // scheduled.
  useEffect(() => {
    if (!isSignedIn) return;

    const subscription = AppState.addEventListener('change', (state) => {
      if (state !== 'active') return;
      void queryClient.invalidateQueries({ queryKey: notificationKeys.reminders });
    });

    return () => subscription.remove();
  }, [isSignedIn, queryClient]);

  // Clear everything on sign-out. Reminders name a specific person's care, and
  // a signed-out phone should say nothing about them.
  //
  // Only on an actual sign-out, though. A cold start is signed-out for as long
  // as the stored session takes to restore, and clearing then would wipe every
  // scheduled reminder on every launch — recoverable while the phone is online,
  // and a silent loss of every reminder while it is not.
  const hasBeenSignedIn = useRef(false);

  useEffect(() => {
    if (isRestoring) return;

    if (isSignedIn) {
      hasBeenSignedIn.current = true;
      return;
    }
    if (!hasBeenSignedIn.current) return;

    hasBeenSignedIn.current = false;
    void clearReminders().catch(() => {});
  }, [isSignedIn, isRestoring]);
}

/**
 * Opens the right screen when a notification is tapped.
 *
 * Separate from the synchronisation above because it is a different concern
 * with a different lifetime: it must be listening before the user taps, whether
 * or not a plan has loaded.
 *
 * A tap can arrive from a lock screen while the app is signed out or still
 * restoring. Navigating then would land on a screen that immediately bounces to
 * sign-in and the destination would be lost, so it is held and honoured once
 * the user is through (plans/phase9.md §26).
 */
export function useReminderTaps(isSignedIn: boolean, isRestoring: boolean) {
  const setPendingDestination = useUIStore((state) => state.setPendingDestination);

  // Read through refs so the listener is subscribed once, at mount, rather than
  // resubscribed on every session change — a tap that arrives during a gap
  // between subscriptions is a notification that silently does nothing.
  const ready = useRef(false);
  ready.current = isSignedIn && !isRestoring;

  useEffect(() => {
    const subscription = Notifications.addNotificationResponseReceivedListener((response) => {
      // A notification can outlive the app version that scheduled it, so an
      // unreadable payload opens nothing rather than crashing the app
      // (plans/phase8.md §37).
      //
      // Two payload shapes reach here: one this device scheduled, and one the
      // server pushed. They are read in that order because a pushed payload is
      // the stricter shape — it names a notification the server holds — and a
      // locally scheduled reminder can never satisfy it.
      const data = response.notification.request.content.data;

      const pushed = readPushPayload(data);
      const destination =
        pushed !== null
          ? notificationDestination(pushed)
          : localDestination(readReminderPayload(data));
      if (destination === null) return;

      if (ready.current) {
        router.push(destination);
        return;
      }
      setPendingDestination(destination);
    });

    return () => subscription.remove();
  }, [setPendingDestination]);
}

/** The destination of a locally scheduled reminder, or null if unreadable. */
function localDestination(payload: ReturnType<typeof readReminderPayload>): Href | null {
  return payload === null ? null : reminderDestination(payload);
}

/**
 * Sends the user where a notification was trying to take them, once they are
 * signed in.
 *
 * The destination is cleared before navigating, so a failed push cannot leave
 * the app looping on the same target.
 */
export function usePendingDestination(isSignedIn: boolean, isRestoring: boolean) {
  const pendingDestination = useUIStore((state) => state.pendingDestination);
  const setPendingDestination = useUIStore((state) => state.setPendingDestination);

  useEffect(() => {
    if (isRestoring || !isSignedIn || pendingDestination === null) return;

    setPendingDestination(null);
    router.push(pendingDestination);
  }, [isSignedIn, isRestoring, pendingDestination, setPendingDestination]);
}
