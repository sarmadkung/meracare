import { createContext, use, useCallback, useEffect, useState, type ReactNode } from 'react';
import { useColorScheme } from 'react-native';

import { secureStorage } from '@/lib/secure-storage';

import { resolveScheme, type ThemePreference } from './preference';
import {
  darkColors,
  elevation,
  lightColors,
  minTouchTarget,
  radii,
  spacing,
  typography,
  type ThemeColors,
} from './tokens';

export interface Theme {
  colors: ThemeColors;
  spacing: typeof spacing;
  radii: typeof radii;
  typography: typeof typography;
  elevation: typeof elevation;
  minTouchTarget: number;
  isDark: boolean;
}

function buildTheme(isDark: boolean): Theme {
  return {
    colors: isDark ? darkColors : lightColors,
    spacing,
    radii,
    typography,
    elevation,
    minTouchTarget,
    isDark,
  };
}

const lightTheme = buildTheme(false);
const darkTheme = buildTheme(true);

const ThemeContext = createContext<Theme>(lightTheme);

/** The appearance choice, and how to change it. */
export interface Appearance {
  preference: ThemePreference;
  setPreference: (preference: ThemePreference) => void;
}

const AppearanceContext = createContext<Appearance>({
  preference: 'system',
  setPreference: () => {},
});

const PREFERENCE_KEY = 'meracare.themePreference';

function isPreference(value: string | null): value is ThemePreference {
  return value === 'system' || value === 'light' || value === 'dark';
}

/**
 * Supplies the active theme.
 *
 * The reader's own choice wins over the phone's setting, and defaults to
 * following it. Dark mode worked before this but could only be reached through
 * the operating system's settings, which is not where anybody looks for an
 * app's appearance.
 *
 * The choice is stored on the device rather than the server: it describes this
 * phone, not this person, and somebody may reasonably want dark on the handset
 * they use at night and light on a tablet.
 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const deviceScheme = useColorScheme();
  const [preference, setPreferenceState] = useState<ThemePreference>('system');

  // Read once at mount. Until it resolves the app follows the phone, which is
  // the same thing the default preference does — so nothing flashes.
  useEffect(() => {
    let active = true;

    void secureStorage
      .getItem(PREFERENCE_KEY)
      .then((stored) => {
        if (active && isPreference(stored)) setPreferenceState(stored);
      })
      .catch(() => {
        // A preference that cannot be read is not worth failing over; the
        // phone's own setting is a good answer.
      });

    return () => {
      active = false;
    };
  }, []);

  const setPreference = useCallback((next: ThemePreference) => {
    // Applied immediately, stored in the background: appearance should change
    // the instant it is tapped, not once a write completes.
    setPreferenceState(next);
    void secureStorage.setItem(PREFERENCE_KEY, next).catch(() => {});
  }, []);

  const theme = resolveScheme(preference, deviceScheme) === 'dark' ? darkTheme : lightTheme;

  return (
    <AppearanceContext value={{ preference, setPreference }}>
      <ThemeContext value={theme}>{children}</ThemeContext>
    </AppearanceContext>
  );
}

/** Returns the active theme. */
export function useTheme(): Theme {
  return use(ThemeContext);
}

/** Returns the appearance choice and a way to change it. */
export function useAppearance(): Appearance {
  return use(AppearanceContext);
}
