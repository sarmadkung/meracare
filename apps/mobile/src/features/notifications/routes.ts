import type { NotificationEntity, ReminderPayload } from '@meracare/contracts';
import type { Href } from 'expo-router';

/**
 * Where tapping a notification should take the user.
 *
 * The server does not decide this, and should not: routes are a property of the
 * app, and a server that named them would have to be redeployed to move a
 * screen. It sends what the reminder is about; this turns that into a
 * destination in the app's existing navigation (plans/phase8.md §15).
 */
export function reminderDestination(payload: ReminderPayload): Href {
  return notificationDestination(payload);
}

/**
 * The little of a notification that decides where it goes.
 *
 * A structural type rather than one of the two concrete ones, so a pushed
 * payload and an inbox row can both be routed by the same function without
 * either being converted into the other.
 */
export interface NotificationTarget {
  entityType: NotificationEntity;
  entityId: string;
  seniorId: string;
}

/**
 * Where tapping a notification should take the user.
 *
 * The same routing as a locally scheduled reminder, because it is the same
 * question. What differs is only that a notification can also be about a care
 * event, which has no screen of its own and belongs in the timeline it is part
 * of.
 *
 * Nothing here checks whether the reader may open the destination, and nothing
 * should: the screen asks the server, which decides. A notification is never
 * permission (plans/phase11.md §§30, 31, 58).
 */
export function notificationDestination({
  entityType,
  entityId,
  seniorId,
}: NotificationTarget): Href {
  switch (entityType) {
    case 'task_instance':
      return { pathname: '/tasks/[taskId]', params: { taskId: entityId } };

    case 'appointment':
      return {
        pathname: '/appointments/[appointmentId]',
        params: { appointmentId: entityId },
      };

    case 'care_event':
      // A care event is one line of a timeline. Opening the timeline shows it
      // in the context of what came before, which is the thing somebody
      // actually wants to see after "Priya recorded a dose".
      return {
        pathname: '/seniors/[seniorId]/activity',
        params: { seniorId },
      };

    case 'medication_dose':
      // A single dose has no screen of its own, and giving it one for the sake
      // of a notification would be a screen nobody navigates to any other way.
      // Today's medication is where the dose is, in the context of the rest of
      // the day's medicine.
      return {
        pathname: '/seniors/[seniorId]/medications',
        params: { seniorId },
      };
  }
}
