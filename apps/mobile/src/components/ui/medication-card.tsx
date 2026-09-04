import type { MedicationDose } from '@genxcare/contracts';
import { doseStatusLabel, doseStatusTone, doseTimeLabel, isDoseOpen } from '@genxcare/contracts';
import { Pressable, View } from 'react-native';

import { useTheme } from '@/theme';

import { Button } from './button';
import { Text } from './text';

export interface MedicationCardProps {
  dose: MedicationDose;
  /** The senior's timezone, so the time reads as their day. */
  timezone: string;
  /** Whether to offer the actions; the API enforces this too. */
  canRecord: boolean;
  onPress: () => void;
  onTake: () => void;
  onSkip: () => void;
}

/**
 * One dose in a list.
 *
 * Designed for an older adult reading it at arm's length: the time, the name
 * and the dose are the largest things on the card, the action is a full-width
 * button rather than a small icon, and the status is spelled out in words as
 * well as carried by colour and by a thicker border — medication status must
 * never depend on colour alone (docs/18, plans/phase5.md §31).
 */
export function MedicationCard({
  dose,
  timezone,
  canRecord,
  onPress,
  onTake,
  onSkip,
}: MedicationCardProps) {
  const theme = useTheme();

  const tone = doseStatusTone(dose.status);
  const toneColor = {
    neutral: theme.colors.textSecondary,
    positive: theme.colors.success,
    attention: theme.colors.warning,
    muted: theme.colors.textMuted,
  }[tone];

  const missed = dose.status === 'missed';

  return (
    <View
      style={{
        backgroundColor: theme.colors.surface,
        borderColor: missed ? theme.colors.warning : theme.colors.border,
        borderWidth: missed ? 2 : 1,
        borderRadius: theme.radii.lg,
        gap: theme.spacing.md,
        padding: theme.spacing.lg,
      }}
    >
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={`${dose.name}${dose.dosage ? `, ${dose.dosage}` : ''}, at ${doseTimeLabel(
          dose,
          timezone,
        )}, ${doseStatusLabel(dose.status)}`}
        onPress={onPress}
        style={{ gap: theme.spacing.xs }}
      >
        <Text variant="sectionHeading">{doseTimeLabel(dose, timezone)}</Text>
        <Text variant="bodyStrong">{dose.name}</Text>

        {dose.dosage ? (
          <Text variant="body" color="secondary">
            {dose.dosage}
          </Text>
        ) : null}

        <Text variant="secondary" style={{ color: toneColor }}>
          {doseStatusLabel(dose.status)}
        </Text>
      </Pressable>

      {isDoseOpen(dose) && canRecord ? (
        <View style={{ gap: theme.spacing.sm }}>
          <Button label="Mark as taken" onPress={onTake} />
          <Button variant="ghost" label="Not this time" onPress={onSkip} />
        </View>
      ) : null}
    </View>
  );
}
