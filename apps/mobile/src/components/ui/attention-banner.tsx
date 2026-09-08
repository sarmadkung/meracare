import { Pressable, View } from 'react-native';

import { useTheme } from '@/theme';

import { Icon } from './icon';
import { Text } from './text';

export interface AttentionBannerProps {
  /** What slipped, named: "Amlodipine was missed". */
  title: string;
  /** When it was due, or the others that also slipped. */
  detail: string;
  onPress?: () => void;
}

/**
 * What has slipped, said at the top of the day.
 *
 * A missed dose deserves to be named in words. Left to the list, it is an amber
 * pill halfway down a screen that has to be scanned to be found — and a person
 * checking on their mother between other things will not scan it.
 */
export function AttentionBanner({ title, detail, onPress }: AttentionBannerProps) {
  const theme = useTheme();

  const content = (
    <>
      <Icon name="bell" color={theme.colors.warning} />

      <View style={{ flex: 1, gap: 2 }}>
        <Text variant="bodyStrong" style={{ color: theme.colors.warning }}>
          {title}
        </Text>
        <Text variant="secondary" style={{ color: theme.colors.warning }}>
          {detail}
        </Text>
      </View>

      {onPress === undefined ? null : <Icon name="chevron" color={theme.colors.warning} />}
    </>
  );

  const ground = {
    alignItems: 'center' as const,
    backgroundColor: theme.colors.warningBackground,
    borderRadius: theme.radii.lg,
    flexDirection: 'row' as const,
    gap: theme.spacing.md,
    minHeight: theme.minTouchTarget,
    padding: theme.spacing.lg,
  };

  if (onPress === undefined) {
    // An announcement rather than a control: there is nowhere to go, and a
    // button that does nothing is worse than plain text.
    return (
      <View accessibilityRole="alert" accessible style={ground}>
        {content}
      </View>
    );
  }

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`${title}, ${detail}`}
      onPress={onPress}
      style={({ pressed }) => [ground, { opacity: pressed ? 0.85 : 1 }]}
    >
      {content}
    </Pressable>
  );
}
