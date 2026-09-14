import { View } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './text';

/**
 * First and last initials.
 *
 * One word gives one letter; nothing usable gives "?", because a blank disc
 * beside a row reads as a rendering failure rather than as missing data.
 */
export function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return '?';

  const first = words[0]?.[0] ?? '';
  const last = words.length > 1 ? (words[words.length - 1]?.[0] ?? '') : '';

  return `${first}${last}`.toUpperCase();
}

export interface AvatarProps {
  name: string;
  size?: number;
}

/**
 * A person, as initials on a brand-tinted disc.
 *
 * Decorative for assistive technology: the name is always rendered beside it,
 * so speaking the initials too would say the same person twice.
 */
export function Avatar({ name, size = 44 }: AvatarProps) {
  const theme = useTheme();

  return (
    <View
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      style={{
        alignItems: 'center',
        backgroundColor: theme.colors.primaryLight,
        borderRadius: theme.radii.pill,
        height: size,
        justifyContent: 'center',
        width: size,
      }}
    >
      <Text variant="bodyStrong" style={{ color: theme.colors.primaryDark }}>
        {initials(name)}
      </Text>
    </View>
  );
}
