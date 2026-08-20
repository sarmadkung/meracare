import {
  ActivityIndicator,
  Image,
  Pressable,
  StyleSheet,
  View,
  type ViewStyle,
} from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './text';

// Google's own "G" mark, taken from their identity branding assets. Google's
// sign-in branding guidelines require the unmodified mark — it must never be
// redrawn, recoloured, or replaced with a lookalike.
const googleMark = require('../../../assets/images/google-g.png') as number;

export interface GoogleButtonProps {
  onPress: () => void;
  loading?: boolean;
  disabled?: boolean;
  style?: ViewStyle;
}

/**
 * "Continue with Google" — the neutral light button from Google's sign-in
 * branding guidelines: the unmodified mark on a white surface with a grey
 * border, so it reads as Google's control rather than a MeraCare one.
 *
 * It keeps the 48dp minimum touch target and visible pressed state that every
 * MeraCare control has (docs/18-visual-theme-and-illustrations.md).
 */
export function GoogleButton({ onPress, loading = false, disabled, style }: GoogleButtonProps) {
  const theme = useTheme();
  const isDisabled = disabled === true || loading;

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel="Continue with Google"
      accessibilityState={{ disabled: isDisabled, busy: loading }}
      disabled={isDisabled}
      onPress={onPress}
      style={({ pressed }) => [
        styles.base,
        {
          minHeight: theme.minTouchTarget,
          paddingHorizontal: theme.spacing.xl,
          borderRadius: theme.radii.md,
          opacity: isDisabled ? 0.5 : pressed ? 0.85 : 1,
        },
        style,
      ]}
    >
      <View style={styles.content}>
        {loading ? (
          <ActivityIndicator color={GOOGLE_TEXT} style={styles.mark} />
        ) : (
          <Image source={googleMark} style={styles.mark} resizeMode="contain" accessible={false} />
        )}
        <Text variant="action" style={styles.label}>
          Continue with Google
        </Text>
      </View>
    </Pressable>
  );
}

// Fixed by Google's branding guidelines, so these are deliberately not theme
// tokens: the button looks the same in light and dark mode.
const GOOGLE_SURFACE = '#FFFFFF';
const GOOGLE_BORDER = '#747775';
const GOOGLE_TEXT = '#1F1F1F';

const styles = StyleSheet.create({
  base: {
    alignItems: 'center',
    backgroundColor: GOOGLE_SURFACE,
    borderColor: GOOGLE_BORDER,
    borderWidth: StyleSheet.hairlineWidth,
    justifyContent: 'center',
  },
  content: {
    alignItems: 'center',
    flexDirection: 'row',
    gap: 12,
    justifyContent: 'center',
  },
  label: {
    color: GOOGLE_TEXT,
  },
  mark: {
    height: 20,
    width: 20,
  },
});
