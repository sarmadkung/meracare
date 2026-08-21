/**
 * Notification types shared between the mobile app and the Go API.
 *
 * These mirror `internal/notifications` and docs/08-notifications-and-background.md.
 * The vocabulary is singular: the server's `ReminderTypes` and the union below
 * must agree, and a test in the API asserts the server half of that
 * (plans/phase8.md §2).
 */

/**
 * The kinds of reminder MeraCare schedules.
 *
 * Exactly the three docs/08 assigns to device-local scheduled reminders. An
 * event that needs telling somebody about right now — a missed dose, an
 * invitation — is a push notification, which is different infrastructure and a
 * later phase.
 */
export const REMINDER_TYPES = [
  'TASK_REMINDER',
  'MEDICATION_REMINDER',
  'APPOINTMENT_REMINDER',
] as const;
export type ReminderType = (typeof REMINDER_TYPES)[number];

/** What a reminder points at, so the app can open the right screen. */
export const REMINDER_ENTITIES = ['task_instance', 'medication_dose', 'appointment'] as const;
export type ReminderEntity = (typeof REMINDER_ENTITIES)[number];

/**
 * One notification the device should schedule locally.
 *
 * Deliberately carries no title and no body. Both are composed on the device
 * from `notification-labels.ts`, which is what keeps a medicine's name out of
 * anything that can appear on a lock screen: the server never sends it, so no
 * screen can accidentally show it (plans/phase8.md §§16, 17, 47).
 */
export interface Reminder {
  /**
   * Stable for as long as the reminder means the same thing, and different the
   * moment it does not. Used directly as the OS notification identifier, which
   * is what makes rescheduling idempotent (plans/phase8.md §25).
   */
  id: string;
  type: ReminderType;

  seniorId: string;
  seniorName: string;
  /** IANA name. The senior's clock decides what "08:00" means, not the reader's. */
  seniorTimezone: string;

  entityType: ReminderEntity;
  entityId: string;

  /** When the care itself is due, ISO-8601. */
  dueAt: string;
  /** When the notification should appear, ISO-8601. */
  fireAt: string;
}

/** A complete reminder plan: everything this user's device should have scheduled. */
export interface ReminderPlan {
  reminders: Reminder[];
  generatedAt: string;
  /**
   * Nothing beyond this instant is planned, so a scheduled notification firing
   * later than this must not be cancelled merely for being absent from the plan.
   */
  horizonEndsAt: string;
}

/**
 * Which categories of notification a user wants.
 *
 * Per user, not per senior: two caregivers looking after the same person
 * routinely want different things (plans/phase8.md §§3, 4).
 */
export interface NotificationPreferences {
  taskReminders: boolean;
  medicationReminders: boolean;
  appointmentReminders: boolean;
  /** Phase 11: a task whose time has passed with nothing recorded. */
  overdueTaskAlerts: boolean;
  /** Phase 11: something somebody else did. */
  careActivity: boolean;
  updatedAt: string;
}

/** One category the settings screen can switch. */
export interface NotificationCategory {
  key: keyof Omit<NotificationPreferences, 'updatedAt'>;
  label: string;
  description: string;
}

/**
 * The switches on the notification settings screen, in order.
 *
 * Worded as what the user gets, not as what the system does: nobody is choosing
 * a `MEDICATION_REMINDER`, they are choosing whether their phone tells them
 * about medicine (plans/phase8.md §18).
 */
export const NOTIFICATION_CATEGORIES: NotificationCategory[] = [
  {
    key: 'medicationReminders',
    label: 'Medication reminders',
    description: 'A reminder shortly before each dose is due.',
  },
  {
    key: 'taskReminders',
    label: 'Care task reminders',
    description: 'A reminder shortly before a care task is due.',
  },
  {
    key: 'appointmentReminders',
    label: 'Appointment reminders',
    description: 'A reminder the day before, and an hour before it starts.',
  },
  {
    key: 'overdueTaskAlerts',
    label: 'Overdue task alerts',
    description: 'A nudge when a care task has passed its time with nothing recorded.',
  },
  {
    key: 'careActivity',
    label: 'Care activity',
    description: 'When someone else records care — a dose given, a task completed.',
  },
];

/** The operating systems a device registration can name. */
export const DEVICE_PLATFORMS = ['ios', 'android', 'web'] as const;
export type DevicePlatform = (typeof DEVICE_PLATFORMS)[number];

/** What the app sends when it announces an installation. */
export interface DeviceRegistration {
  deviceId: string;
  platform: DevicePlatform;
  /** Absent until the user has granted OS notification permission. */
  pushToken?: string;
  appVersion?: string;
}

/**
 * A registered installation, as the API describes it.
 *
 * There is no push token field, and adding one would be a security regression:
 * the client that registered the token already has it, and no other client has
 * any business reading it (plans/phase8.md §8).
 */
export interface RegisteredDevice {
  id: string;
  deviceId: string;
  platform: DevicePlatform;
  appVersion: string;
  active: boolean;
  lastSeenAt: string;
  pushTokenRegistered: boolean;
}

/**
 * The kinds of notification MeraCare delivers and keeps.
 *
 * A superset of `REMINDER_TYPES`: the three a device can schedule for itself,
 * plus the two only a server can know about. Mirrors
 * `internal/notifications.Types` and the CHECK constraint on
 * `notifications.notification_type` (plans/phase11.md §5).
 */
export const NOTIFICATION_TYPES = [
  'MEDICATION_REMINDER',
  'APPOINTMENT_REMINDER',
  'TASK_REMINDER',
  'TASK_OVERDUE',
  'CARE_ACTIVITY',
] as const;
export type NotificationType = (typeof NOTIFICATION_TYPES)[number];

/** What a notification points at, so the app can open the right screen. */
export const NOTIFICATION_ENTITIES = [
  'task_instance',
  'medication_dose',
  'appointment',
  'care_event',
] as const;
export type NotificationEntity = (typeof NOTIFICATION_ENTITIES)[number];

/**
 * One row of the notification inbox.
 *
 * Unlike a `Reminder`, this one *does* carry its words — because they are the
 * words that were sent. A reminder is a plan the device renders; a notification
 * is a record of something that happened, and re-composing its sentence later
 * would make yesterday's inbox change when an appointment moves
 * (plans/phase11.md §6).
 *
 * The wording still says nothing medical: the same lock-screen privacy policy
 * produced it, on the server, in `internal/notifications/wording.go`
 * (plans/phase11.md §48).
 */
export interface Notification {
  id: string;
  type: NotificationType;

  title: string;
  body: string;

  seniorId: string;
  entityType: NotificationEntity;
  entityId: string;

  /** What the notification is for, ISO-8601. Also the inbox's sort order. */
  occurredAt: string;
  read: boolean;
  /** Empty while unread. */
  readAt: string;
}

/** One page of the inbox, newest first. */
export interface NotificationInbox {
  items: Notification[];
  nextCursor: string | null;
  /**
   * Every unread notification, not just the ones on this page — it travels with
   * the page so a badge and the list it labels cannot disagree
   * (plans/phase11.md §61).
   */
  unreadCount: number;
}

/** What marking everything read changed. */
export interface MarkAllReadResult {
  markedRead: number;
  unreadCount: number;
}

/**
 * The identifiers a push notification carries.
 *
 * Identifiers only, and never treated as permission. Tapping a notification
 * opens a screen, and that screen asks the server for the thing under the
 * reader's own authorization — so a notification that outlived somebody's
 * access opens nothing (plans/phase11.md §§31, 58).
 */
export interface PushPayload {
  notificationId: string;
  type: NotificationType;
  seniorId: string;
  entityType: NotificationEntity;
  entityId: string;
}

/**
 * Reads a payload off a push notification the user tapped.
 *
 * Returns null rather than throwing for anything unrecognised, for the same
 * reason `readReminderPayload` does: a notification can outlive the app version
 * that sent it, and a crash on opening a stale one is a worse bug than a tap
 * that does nothing.
 */
export function readPushPayload(data: unknown): PushPayload | null {
  if (typeof data !== 'object' || data === null) return null;

  const candidate = data as Record<string, unknown>;
  const required = ['notificationId', 'type', 'seniorId', 'entityType', 'entityId'] as const;

  for (const key of required) {
    if (typeof candidate[key] !== 'string' || candidate[key] === '') return null;
  }

  if (!NOTIFICATION_TYPES.includes(candidate.type as NotificationType)) return null;
  if (!NOTIFICATION_ENTITIES.includes(candidate.entityType as NotificationEntity)) return null;

  return {
    notificationId: candidate.notificationId as string,
    type: candidate.type as NotificationType,
    seniorId: candidate.seniorId as string,
    entityType: candidate.entityType as NotificationEntity,
    entityId: candidate.entityId as string,
  };
}
