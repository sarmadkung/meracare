import { Image, StyleSheet, View, type ImageSourcePropType, type ViewStyle } from 'react-native';

import { useTheme } from '@/theme';

export type IllustrationName =
  'addSenior' | 'allCaughtUp' | 'careCircle' | 'communication' | 'welcome';

export interface IllustrationProps {
  /** Stable semantic name; screens do not depend on vendor filenames. */
  name: IllustrationName;
  height?: number;
  /**
   * Omit when nearby text already communicates the same meaning. Supplying a
   * label makes the image visible to assistive technology.
   */
  accessibilityLabel?: string;
  style?: ViewStyle;
}

const sources: Record<IllustrationName, ImageSourcePropType> = {
  addSenior: require('../../../assets/illustrations/undraw/add-senior.png'),
  allCaughtUp: require('../../../assets/illustrations/undraw/all-caught-up.png'),
  careCircle: require('../../../assets/illustrations/undraw/care-circle.png'),
  communication: require('../../../assets/illustrations/undraw/communication.png'),
  welcome: require('../../../assets/illustrations/undraw/welcome-team.png'),
};

/** A locally bundled, consistently framed GenXcare illustration. */
export function Illustration({ name, height = 180, accessibilityLabel, style }: IllustrationProps) {
  const theme = useTheme();

  return (
    <View
      pointerEvents="none"
      style={[
        styles.frame,
        {
          backgroundColor: theme.colors.surfaceMuted,
          borderRadius: theme.radii.lg,
          height,
          padding: theme.spacing.md,
        },
        style,
      ]}
    >
      <Image
        source={sources[name]}
        resizeMode="contain"
        accessible={Boolean(accessibilityLabel)}
        accessibilityLabel={accessibilityLabel}
        accessibilityIgnoresInvertColors
        style={styles.image}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  frame: {
    alignItems: 'center',
    justifyContent: 'center',
    overflow: 'hidden',
    width: '100%',
  },
  image: {
    height: '100%',
    width: '100%',
  },
});
