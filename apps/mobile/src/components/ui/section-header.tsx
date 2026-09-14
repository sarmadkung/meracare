import type { ReactNode } from 'react';
import { View } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './text';

export interface SectionHeaderProps {
  title: string;
  /** An optional trailing control, such as "See all". */
  action?: ReactNode;
}

/**
 * The eyebrow above a group of rows.
 *
 * A real heading, so a screen reader can jump between sections instead of
 * reading the screen top to bottom. The title is uppercased through
 * `textTransform` rather than in the string, so what is spoken stays sentence
 * case — "UP NEXT" is read letter by letter by some screen readers.
 */
export function SectionHeader({ title, action }: SectionHeaderProps) {
  const theme = useTheme();

  return (
    <View
      style={{
        alignItems: 'center',
        flexDirection: 'row',
        justifyContent: 'space-between',
        marginTop: theme.spacing.sm,
      }}
    >
      <Text
        accessibilityRole="header"
        variant="label"
        color="secondary"
        style={{ textTransform: 'uppercase' }}
      >
        {title}
      </Text>
      {action}
    </View>
  );
}
