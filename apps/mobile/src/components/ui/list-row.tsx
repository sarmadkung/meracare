import { router, type Href } from 'expo-router';
import type { ReactNode } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { useTheme } from '@/theme';

import { Icon } from './icon';
import { Text } from './text';

export interface ListRowProps {
  title: string;
  subtitle?: string;
  /** Where tapping goes. Navigation happens here, never by wrapping this row. */
  href?: Href;
  onPress?: () => void;
  /** Avatar, icon tile, or anything else that identifies the row. */
  leading?: ReactNode;
  /** Replaces the default chevron — a badge or an action, say. */
  trailing?: ReactNode;
  /** Overrides the spoken name, which is otherwise "title, subtitle". */
  accessibilityLabel?: string;
}

/**
 * One tappable row in a list.
 *
 * It navigates itself, and that is the point. Wrapping a styled row in
 * `<Link asChild>` looks equivalent but is not: expo-router clones the child
 * and passes its own `style`, silently discarding the child's, so the row
 * renders as bare unstyled text with the chevron dropped onto its own line.
 * That bug shipped. Owning navigation here means no caller can reintroduce it.
 */
export function ListRow({
  title,
  subtitle,
  href,
  onPress,
  leading,
  trailing,
  accessibilityLabel,
}: ListRowProps) {
  const theme = useTheme();

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={
        accessibilityLabel ?? (subtitle === undefined ? title : `${title}, ${subtitle}`)
      }
      onPress={() => {
        if (onPress !== undefined) onPress();
        else if (href !== undefined) router.push(href);
      }}
      style={({ pressed }) => [
        styles.base,
        {
          backgroundColor: theme.colors.surface,
          borderColor: theme.colors.border,
          borderRadius: theme.radii.lg,
          gap: theme.spacing.md,
          minHeight: theme.minTouchTarget,
          opacity: pressed ? 0.85 : 1,
          padding: theme.spacing.lg,
        },
        theme.elevation.card,
      ]}
    >
      {leading}

      <View style={{ flex: 1, gap: theme.spacing.xs }}>
        <Text variant="bodyStrong">{title}</Text>
        {subtitle === undefined ? null : (
          <Text variant="secondary" color="secondary">
            {subtitle}
          </Text>
        )}
      </View>

      {trailing ?? <Icon name="chevron" color={theme.colors.textMuted} />}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    alignItems: 'center',
    borderWidth: StyleSheet.hairlineWidth,
    flexDirection: 'row',
  },
});
