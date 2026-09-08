import { Stack } from 'expo-router';
import { Pressable, StyleSheet, View } from 'react-native';

import { Icon, Screen, Text } from '@/components/ui';
import {
  themePreferenceLabel,
  themePreferences,
  useAppearance,
  useTheme,
  type ThemePreference,
} from '@/theme';

/**
 * Appearance (docs/13-mvp-screen-map.md, screen 26).
 *
 * Dark mode has always worked, but the only way to reach it was the phone's own
 * settings — which is not where anybody looks for how an app should look.
 *
 * "Match my phone" stays first and is the default: following the device is the
 * right behaviour, and overriding it should be a deliberate act.
 */
export default function AppearanceScreen() {
  const theme = useTheme();
  const { preference, setPreference } = useAppearance();

  return (
    <Screen scrollable variant="list">
      <Stack.Screen options={{ title: 'Appearance' }} />

      {/*
        A radio group rather than a switch: there are three states, and "dark
        off" would be an ambiguous way to say "follow the phone".
      */}
      <View accessibilityRole="radiogroup" style={{ gap: theme.spacing.sm }}>
        {themePreferences.map((option) => (
          <Choice
            key={option}
            option={option}
            selected={option === preference}
            onSelect={() => setPreference(option)}
          />
        ))}
      </View>

      <Text variant="secondary" color="secondary">
        Matching your phone follows its own light and dark schedule, including any night setting you
        have.
      </Text>
    </Screen>
  );
}

function Choice({
  option,
  selected,
  onSelect,
}: {
  option: ThemePreference;
  selected: boolean;
  onSelect: () => void;
}) {
  const theme = useTheme();

  return (
    <Pressable
      accessibilityRole="radio"
      accessibilityState={{ checked: selected }}
      accessibilityLabel={themePreferenceLabel(option)}
      onPress={onSelect}
      style={({ pressed }) => [
        styles.row,
        {
          backgroundColor: theme.colors.surface,
          borderColor: selected ? theme.colors.primary : theme.colors.border,
          borderRadius: theme.radii.lg,
          gap: theme.spacing.md,
          minHeight: theme.minTouchTarget,
          opacity: pressed ? 0.85 : 1,
          padding: theme.spacing.lg,
        },
        theme.elevation.card,
      ]}
    >
      <Text variant="bodyStrong" style={{ flex: 1 }}>
        {themePreferenceLabel(option)}
      </Text>

      {/*
        The tick is decorative: the selected state is already announced through
        accessibilityState, and a second announcement would just repeat it.
      */}
      {selected ? <Icon name="task" color={theme.colors.primary} /> : null}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  row: {
    alignItems: 'center',
    borderWidth: 1,
    flexDirection: 'row',
  },
});
