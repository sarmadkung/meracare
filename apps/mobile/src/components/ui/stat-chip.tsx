import { View } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './text';

export type StatTone = 'brand' | 'neutral' | 'warning';

export interface StatChipProps {
  value: string;
  label: string;
  tone?: StatTone;
}

/**
 * One number and what it counts.
 *
 * Grouped for assistive technology because "3/4" and "Meds" are a single fact;
 * announced apart they are two fragments that mean nothing. The label is always
 * text, never colour alone — a person who cannot tell amber from teal still has
 * to know that one dose is overdue.
 */
export function StatChip({ value, label, tone = 'neutral' }: StatChipProps) {
  const theme = useTheme();

  const grounds: Record<StatTone, string> = {
    brand: theme.colors.primarySubtle,
    neutral: theme.colors.surfaceMuted,
    warning: theme.colors.warningBackground,
  };

  const values: Record<StatTone, string> = {
    brand: theme.colors.primary,
    neutral: theme.colors.textPrimary,
    warning: theme.colors.warning,
  };

  return (
    <View
      accessible
      accessibilityLabel={`${value} ${label}`}
      style={{
        backgroundColor: grounds[tone],
        borderRadius: theme.radii.md,
        flex: 1,
        gap: theme.spacing.xs,
        padding: theme.spacing.md,
      }}
    >
      <Text variant="bodyStrong" style={{ color: values[tone] }}>
        {value}
      </Text>
      <Text variant="label" color={tone === 'warning' ? 'warning' : 'secondary'}>
        {label}
      </Text>
    </View>
  );
}
