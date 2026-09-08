import { clearReminders } from '@/features/notifications/scheduler';
import { deviceId } from '@/features/notifications/device';
import { syncQueuedOperations } from '@/features/sync/replay';
import { apiRequest } from '@/lib/api-client';
import { ApiError } from '@/lib/api-error';
import { clearOfflineData, queuedOperationCount } from '@/lib/offline/database';

/** A sign-out failure whose text is safe to show directly. */
export class SignOutPreparationError extends Error {
  constructor(message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = 'SignOutPreparationError';
  }
}

/**
 * Removes everything tied to the current account before its token disappears.
 *
 * Care mutations are drained first. Signing out with one still queued would
 * either erase a real action or let the next person on the device replay it as
 * themselves. Device deactivation must also precede Supabase sign-out because
 * its endpoint needs the current access token.
 */
export async function prepareForSignOut(): Promise<void> {
  try {
    await syncQueuedOperations();
  } catch (cause) {
    throw new SignOutPreparationError(
      'Your offline care updates could not be checked. Connect to the internet and try again.',
      { cause },
    );
  }

  if ((await queuedOperationCount()) > 0) {
    throw new SignOutPreparationError(
      'Some offline care updates still need attention. Connect to the internet and try again.',
    );
  }

  try {
    const id = await deviceId();
    await apiRequest<void>(`/notifications/devices/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    });
  } catch (cause) {
    // A device the server has no record of is already unreachable, which is the
    // whole point of the call — so 404 is the desired end state, not a failure.
    // Registration happens once per sign-in and is never retried, so a phone
    // that was offline at that moment would otherwise be unable to sign out at
    // all, permanently (plans/phase8.md §25).
    if (!(cause instanceof ApiError && cause.status === 404)) {
      throw new SignOutPreparationError(
        'This device could not be disconnected securely. Connect to the internet and try again.',
        { cause },
      );
    }
  }

  await clearReminders();
  await clearOfflineData();
}
