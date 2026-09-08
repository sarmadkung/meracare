import { createContext, use, type ReactNode } from 'react';
import { useColorScheme } from 'react-native';

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

/**
 * Supplies the active theme, following the OS appearance setting.
 *
 * A user-facing appearance preference belongs in the Zustand UI store later;
 * for now the platform setting is the single source of truth.
 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const colorScheme = useColorScheme();
  const theme = colorScheme === 'dark' ? darkTheme : lightTheme;

  return <ThemeContext value={theme}>{children}</ThemeContext>;
}

/** Returns the active theme. */
export function useTheme(): Theme {
  return use(ThemeContext);
}
