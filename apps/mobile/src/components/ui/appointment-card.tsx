import type { Appointment } from '@genxcare/contracts';
import {
  appointmentPlaceLabel,
  appointmentStatusLabel,
  appointmentStatusTone,
  appointmentWhenLabel,
} from '@genxcare/contracts';
import { Pressable } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './text';

export interface AppointmentCardProps {
  appointment: Appointment;
  /** The senior's timezone, so the time reads as their day. */
  timezone: string;
  onPress: () => void;
}

/**
 * One appointment in a list.
 *
 * Designed for an older adult reading it at arm's length: the time is the
 * largest thing on the card, then where they are going, because those are the
 * two facts somebody checks on the morning of a visit (plans/phase6.md §16).
 *
 * A settled appointment is distinguished three ways at once — the status in
 * words, a tone, and a faded card — because status must never depend on colour
 * alone (docs/18, plans/phase6.md §31). Cancelled is faded rather than hidden:
 * a visit that quietly disappeared would look like one nobody had mentioned.
 */
export function AppointmentCard({ appointment, timezone, onPress }: AppointmentCardProps) {
  const theme = useTheme();

  const tone = appointmentStatusTone(appointment.status);
  const toneColor = {
    neutral: theme.colors.textSecondary,
    positive: theme.colors.success,
    muted: theme.colors.textMuted,
  }[tone];

  const cancelled = appointment.status === 'cancelled';
  const place = appointmentPlaceLabel(appointment);

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`${appointment.title}, ${appointmentWhenLabel(
        appointment,
        timezone,
      )}${place ? `, ${place}` : ''}, ${appointmentStatusLabel(appointment.status)}`}
      onPress={onPress}
      style={{
        backgroundColor: theme.colors.surface,
        borderColor: theme.colors.border,
        borderWidth: 1,
        borderRadius: theme.radii.lg,
        gap: theme.spacing.xs,
        opacity: cancelled ? 0.6 : 1,
        padding: theme.spacing.lg,
      }}
    >
      <Text variant="sectionHeading">{appointmentWhenLabel(appointment, timezone)}</Text>
      <Text variant="bodyStrong">{appointment.title}</Text>

      {place ? (
        <Text variant="body" color="secondary">
          {place}
        </Text>
      ) : null}

      <Text variant="secondary" style={{ color: toneColor }}>
        {appointmentStatusLabel(appointment.status)}
      </Text>
    </Pressable>
  );
}
