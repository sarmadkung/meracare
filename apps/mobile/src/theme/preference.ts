/**
 * What the reader has asked the app to look like.
 *
 * "System" is the default and stays available as a choice: an app that ignores
 * the phone's night setting is a worse citizen than one offering no control at
 * all, so overriding it has to be deliberate.
 */
export type ThemePreference = 'system' | 'light' | 'dark';

/** Every preference, in the order the settings screen offers them. */
export const themePreferences: ThemePreference[] = ['system', 'light', 'dark'];

/** Plain-language label for a preference. */
export function themePreferenceLabel(preference: ThemePreference): string {
  switch (preference) {
    case 'light':
      return 'Light';
    case 'dark':
      return 'Dark';
    default:
      return 'Match my phone';
  }
}

/**
 * Decides which palette to draw with.
 *
 * A device that reports nothing is treated as light, which docs/18 names the
 * primary mode.
 */
export function resolveScheme(
  preference: ThemePreference,
  // Widened because React Native also reports "unspecified", which means the
  // same thing as knowing nothing.
  deviceScheme: string | null | undefined,
): 'light' | 'dark' {
  if (preference !== 'system') return preference;
  return deviceScheme === 'dark' ? 'dark' : 'light';
}
