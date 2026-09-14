import type { PermissionLabel } from '@genxcare/contracts';
import { Pressable, StyleSheet, View } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './text';

export interface PermissionToggleProps {
  entry: PermissionLabel;
  selected: boolean;
  /** Permissions the granter cannot confer are shown, but not selectable. */
  disabled?: boolean;
  onToggle: () => void;
}

/**
 * One permission, described in plain language, as a checkbox row.
 *
 * The raw identifier is never shown: a person deciding what a caregiver may do
 * should read "Record medications", not `medications.record`.
 */
export function PermissionToggle({
  entry,
  selected,
  disabled = false,
  onToggle,
}: PermissionToggleProps) {
  const theme = useTheme();

  return (
    <Pressable
      accessibilityRole="checkbox"
      accessibilityState={{ checked: selected, disabled }}
      accessibilityLabel={`${entry.label}. ${entry.description}`}
      accessibilityHint={disabled ? 'You cannot grant this permission.' : undefined}
      disabled={disabled}
      onPress={onToggle}
      style={({ pressed }) => [
        styles.row,
        {
          minHeight: theme.minTouchTarget,
          padding: theme.spacing.md,
          borderRadius: theme.radii.md,
          gap: theme.spacing.md,
          backgroundColor: selected ? theme.colors.primaryLight : theme.colors.surface,
          borderColor: selected ? theme.colors.primary : theme.colors.border,
          opacity: disabled ? 0.45 : pressed ? 0.85 : 1,
        },
      ]}
    >
      {/* A box plus a tick: selection is never signalled by colour alone. */}
      <View
        style={[
          styles.box,
          {
            borderRadius: theme.radii.sm,
            borderColor: selected ? theme.colors.primary : theme.colors.border,
            backgroundColor: selected ? theme.colors.primary : 'transparent',
          },
        ]}
      >
        {selected ? (
          <Text variant="bodyStrong" style={{ color: theme.colors.onPrimary }}>
            ✓
          </Text>
        ) : null}
      </View>

      <View style={{ flex: 1, gap: theme.spacing.xs }}>
        <Text variant="bodyStrong">{entry.label}</Text>
        <Text variant="secondary" color="secondary">
          {entry.description}
        </Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  box: {
    alignItems: 'center',
    borderWidth: 2,
    height: 28,
    justifyContent: 'center',
    width: 28,
  },
  row: {
    alignItems: 'center',
    borderWidth: StyleSheet.hairlineWidth,
    flexDirection: 'row',
  },
});
