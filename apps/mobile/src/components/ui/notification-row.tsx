import type { Notification } from '@meracare/contracts';
import { timeInTimezone } from '@meracare/contracts';
import { Pressable, View } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './text';

export interface NotificationRowProps {
  notification: Notification;
  /** The timezone the arrival time is read in. */
  timezone: string;
  onPress: () => void;
}

/**
 * One row of the notification inbox.
 *
 * The title and body are the server's, unchanged. They are the words that were
 * sent, and re-writing them here would mean the inbox and the lock screen could
 * disagree about what MeraCare said (plans/phase11.md §6).
 *
 * Unread is marked twice over — a dot and a heavier title — because a state
 * carried by colour alone is a state some readers cannot see
 * (docs/18-visual-theme-and-illustrations.md).
 */
export function NotificationRow({ notification, timezone, onPress }: NotificationRowProps) {
  const theme = useTheme();
  const time = timeInTimezone(notification.occurredAt, timezone);

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`${notification.read ? '' : 'Unread. '}${notification.title}. ${
        notification.body
      } ${time}.`}
      onPress={onPress}
      style={({ pressed }) => ({
        backgroundColor: theme.colors.surface,
        borderColor: theme.colors.border,
        borderRadius: theme.radii.md,
        borderWidth: 1,
        flexDirection: 'row',
        gap: theme.spacing.md,
        minHeight: theme.minTouchTarget,
        opacity: pressed ? 0.85 : 1,
        padding: theme.spacing.lg,
      })}
    >
      <View
        accessible={false}
        style={{
          backgroundColor: notification.read ? 'transparent' : theme.colors.primary,
          borderRadius: theme.radii.pill,
          height: 10,
          marginTop: 6,
          width: 10,
        }}
      />

      <View style={{ flex: 1, gap: theme.spacing.xs }}>
        <Text variant={notification.read ? 'body' : 'bodyStrong'}>{notification.title}</Text>
        <Text variant="body" color="secondary">
          {notification.body}
        </Text>
        <Text variant="secondary" color="muted">
          {time}
        </Text>
      </View>
    </Pressable>
  );
}
