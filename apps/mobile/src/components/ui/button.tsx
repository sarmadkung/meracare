import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  View,
  type PressableProps,
  type ViewStyle,
} from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './text';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';

export interface ButtonProps extends Omit<PressableProps, 'children' | 'style'> {
  label: string;
  variant?: ButtonVariant;
  loading?: boolean;
  style?: ViewStyle;
}

/**
 * Primary action control.
 *
 * Meets the 48dp minimum touch target from docs/18 and always shows a visible
 * pressed state, so the affordance never depends on colour alone.
 */
export function Button({
  label,
  variant = 'primary',
  loading = false,
  disabled,
  style,
  ...rest
}: ButtonProps) {
  const theme = useTheme();
  const isDisabled = disabled === true || loading;

  const background = {
    primary: theme.colors.primary,
    secondary: theme.colors.primaryLight,
    ghost: 'transparent',
    danger: theme.colors.danger,
  }[variant];

  // Solid variants put text on the brand/danger fill; the rest sit on the page
  // background and use the brand colour for the label.
  const isSolid = variant === 'primary' || variant === 'danger';
  const labelColor = isSolid ? theme.colors.onPrimary : theme.colors.primary;

  return (
    <Pressable
      accessibilityRole="button"
      // The spinner replaces the label while loading, which would otherwise
      // leave the control with no spoken name at the exact moment a person
      // wants to know what it is doing.
      accessibilityLabel={label}
      accessibilityState={{ disabled: isDisabled, busy: loading }}
      disabled={isDisabled}
      style={({ pressed }) => [
        styles.base,
        {
          minHeight: theme.minTouchTarget,
          paddingHorizontal: theme.spacing.xl,
          borderRadius: theme.radii.md,
          backgroundColor: background,
          borderColor: variant === 'ghost' ? theme.colors.border : 'transparent',
          opacity: isDisabled ? 0.5 : pressed ? 0.85 : 1,
        },
        style,
      ]}
      {...rest}
    >
      <View style={styles.content}>
        {loading ? (
          <ActivityIndicator color={labelColor} />
        ) : (
          <Text variant="action" style={{ color: labelColor }}>
            {label}
          </Text>
        )}
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: StyleSheet.hairlineWidth,
  },
  content: {
    alignItems: 'center',
    flexDirection: 'row',
    gap: 8,
    justifyContent: 'center',
  },
});
