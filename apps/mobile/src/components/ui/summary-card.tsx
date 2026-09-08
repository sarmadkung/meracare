import { router, type Href } from 'expo-router';
import { Pressable, StyleSheet, View } from 'react-native';

import { useTheme } from '@/theme';

import { Avatar } from './avatar';
import { Icon } from './icon';
import { StatChip, type StatTone } from './stat-chip';
import { Text } from './text';

export interface SummaryStat {
  value: string;
  label: string;
  tone?: StatTone;
}

export interface SummaryCardProps {
  name: string;
  role: string;
  href: Href;
  stats: SummaryStat[];
}

/**
 * One person in a care circle, with how their day is going.
 *
 * This replaces the bare name-and-chevron row. A caregiver opens the app to
 * find out whether anything needs them, and a list of names cannot answer that
 * — so the answer is on the card rather than one tap further in.
 *
 * The whole card is one button, and one accessible name. Making the stats
 * separately focusable would put a screen reader through four stops to learn
 * what a sighted user takes in at once.
 */
export function SummaryCard({ name, role, href, stats }: SummaryCardProps) {
  const theme = useTheme();

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`${name}, ${role}`}
      onPress={() => router.push(href)}
      style={({ pressed }) => [
        styles.base,
        {
          backgroundColor: theme.colors.surface,
          borderColor: theme.colors.border,
          borderRadius: theme.radii.lg,
          gap: theme.spacing.md,
          opacity: pressed ? 0.85 : 1,
          padding: theme.spacing.lg,
        },
        theme.elevation.card,
      ]}
    >
      <View style={{ alignItems: 'center', flexDirection: 'row', gap: theme.spacing.md }}>
        <Avatar name={name} />

        <View style={{ flex: 1, gap: theme.spacing.xs }}>
          <Text variant="bodyStrong">{name}</Text>
          <Text variant="secondary" color="secondary">
            {role}
          </Text>
        </View>

        <Icon name="chevron" color={theme.colors.textMuted} />
      </View>

      {stats.length === 0 ? null : (
        <View style={{ flexDirection: 'row', gap: theme.spacing.sm }}>
          {stats.map((stat) => (
            <StatChip key={stat.label} value={stat.value} label={stat.label} tone={stat.tone} />
          ))}
        </View>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    borderWidth: StyleSheet.hairlineWidth,
  },
});
