import { ScrollView, StyleSheet, View, type ViewProps } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { useTheme } from '@/theme';

/**
 * How much room this kind of content needs between its parts.
 *
 * Lists sit tighter because their cards carry their own padding; forms need
 * room between fields; a detail screen reads as sections and wants the most.
 * One uniform gap across all three is what left lists looking loose and
 * unrelated to each other.
 */
export type ScreenVariant = 'list' | 'form' | 'detail';

export interface ScreenProps extends ViewProps {
  /** Wraps the content in a ScrollView. Use for forms and long content. */
  scrollable?: boolean;
  /** Defaults to `form`, which is the rhythm every existing screen was built against. */
  variant?: ScreenVariant;
}

/**
 * Page container: applies the themed background and the safe-area insets that
 * every screen needs.
 */
export function Screen({
  scrollable = false,
  variant = 'form',
  style,
  children,
  ...rest
}: ScreenProps) {
  const theme = useTheme();
  const insets = useSafeAreaInsets();

  const gaps: Record<ScreenVariant, number> = {
    list: theme.spacing.md,
    form: theme.spacing.lg,
    detail: theme.spacing.xl,
  };

  const contentStyle = [
    {
      padding: theme.spacing.lg,
      paddingTop: insets.top + theme.spacing.lg,
      paddingBottom: insets.bottom + theme.spacing.lg,
      gap: gaps[variant],
    },
    style,
  ];

  if (scrollable) {
    return (
      <ScrollView
        style={[styles.fill, { backgroundColor: theme.colors.background }]}
        contentContainerStyle={contentStyle}
        keyboardShouldPersistTaps="handled"
        {...rest}
      >
        {children}
      </ScrollView>
    );
  }

  return (
    <View
      style={[styles.fill, { backgroundColor: theme.colors.background }, contentStyle]}
      {...rest}
    >
      {children}
    </View>
  );
}

const styles = StyleSheet.create({
  fill: {
    flex: 1,
  },
});
