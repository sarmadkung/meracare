import Ionicons from '@expo/vector-icons/Ionicons';
import type { ComponentProps } from 'react';

import { useTheme } from '@/theme';

/**
 * The app's icon vocabulary.
 *
 * Screens name what they mean, never a glyph. This is the same containment
 * `Illustration` gives unDraw assets: the set can be swapped in one file, and
 * no screen can invent a one-off icon that belongs to nothing. Before this
 * existed the app had no icons at all — a chevron was the literal text
 * character `›`, which is most of why it read as a prototype.
 */
export type IconName =
  | 'chevron'
  | 'bell'
  | 'pill'
  | 'calendar'
  | 'people'
  | 'settings'
  | 'today'
  | 'task'
  | 'plus';

const glyphs: Record<IconName, ComponentProps<typeof Ionicons>['name']> = {
  chevron: 'chevron-forward',
  bell: 'notifications-outline',
  pill: 'medkit-outline',
  calendar: 'calendar-outline',
  people: 'people-outline',
  settings: 'settings-outline',
  today: 'home-outline',
  task: 'checkmark-circle-outline',
  plus: 'add',
};

export interface IconProps {
  name: IconName;
  size?: number;
  /** Defaults to the secondary text colour. Pass a theme colour, never a literal. */
  color?: string;
  /**
   * Omit when adjacent text already says the same thing — otherwise a screen
   * reader announces it twice.
   */
  accessibilityLabel?: string;
}

export function Icon({ name, size = 20, color, accessibilityLabel }: IconProps) {
  const theme = useTheme();
  const decorative = accessibilityLabel === undefined;

  return (
    <Ionicons
      name={glyphs[name]}
      size={size}
      color={color ?? theme.colors.textSecondary}
      accessibilityElementsHidden={decorative}
      importantForAccessibility={decorative ? 'no-hide-descendants' : 'yes'}
      accessibilityLabel={accessibilityLabel}
    />
  );
}
