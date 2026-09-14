import type { CareTask } from '@genxcare/contracts';
import { isOpen, statusLabel, statusTone, taskTimeLabel } from '@genxcare/contracts';
import { Pressable, View } from 'react-native';

import { useTheme } from '@/theme';

import { Button } from './button';
import { Text } from './text';

export interface TaskCardProps {
  task: CareTask;
  /** The senior's timezone, so the due time reads as their day. */
  timezone: string;
  /** Who it is for, when the reader is not the only person involved. */
  assigneeName?: string | null;
  /** Whether to offer the completion action; the API enforces this too. */
  canComplete: boolean;
  onPress: () => void;
  onComplete: () => void;
  onSkip: () => void;
}

/**
 * One task in a list.
 *
 * Designed for an older adult reading it at arm's length: the time and the
 * title are the largest things on the card, the action is a full-width button
 * rather than a small icon, and the status is spelled out in words as well as
 * carried by colour (docs/18, plans/phase4.md §34).
 */
export function TaskCard({
  task,
  timezone,
  assigneeName,
  canComplete,
  onPress,
  onComplete,
  onSkip,
}: TaskCardProps) {
  const theme = useTheme();

  const tone = statusTone(task.status);
  const toneColor = {
    neutral: theme.colors.textSecondary,
    positive: theme.colors.success,
    attention: theme.colors.warning,
    muted: theme.colors.textMuted,
  }[tone];

  const open = isOpen(task);

  return (
    <View
      style={{
        backgroundColor: theme.colors.surface,
        borderColor: task.status === 'overdue' ? theme.colors.warning : theme.colors.border,
        // A thicker edge, not only a different colour, so the overdue state is
        // visible to somebody who cannot distinguish the two.
        borderWidth: task.status === 'overdue' ? 2 : 1,
        borderRadius: theme.radii.lg,
        gap: theme.spacing.md,
        padding: theme.spacing.lg,
      }}
    >
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={`${task.title}, due at ${taskTimeLabel(task, timezone)}, ${statusLabel(
          task.status,
        )}`}
        onPress={onPress}
        style={{ gap: theme.spacing.xs }}
      >
        <Text variant="sectionHeading">{taskTimeLabel(task, timezone)}</Text>
        <Text variant="bodyStrong">{task.title}</Text>

        {assigneeName ? (
          <Text variant="secondary" color="secondary">
            For {assigneeName}
          </Text>
        ) : null}

        <Text variant="secondary" style={{ color: toneColor }}>
          {statusLabel(task.status)}
        </Text>
      </Pressable>

      {open && canComplete ? (
        <View style={{ gap: theme.spacing.sm }}>
          <Button label="Mark as done" onPress={onComplete} />
          <Button variant="ghost" label="Not this time" onPress={onSkip} />
        </View>
      ) : null}
    </View>
  );
}
