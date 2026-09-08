import { resolveScheme, type ThemePreference } from '../preference';

/**
 * Dark mode existed but could only be reached through the operating system's
 * own settings, which is not somewhere people look for an app's appearance.
 *
 * "System" stays the default: an app that ignores the phone's night setting is
 * a worse citizen than one with no control at all.
 */

it.each<[ThemePreference, string | null, 'light' | 'dark']>([
  ['system', 'dark', 'dark'],
  ['system', 'light', 'light'],
  ['light', 'dark', 'light'],
  ['dark', 'light', 'dark'],
])('preference %p with the phone on %p renders %p', (preference, device, expected) => {
  expect(resolveScheme(preference, device)).toBe(expected);
});

/** A phone that reports nothing is treated as light, which is the primary mode. */
it('falls back to light when the device says nothing', () => {
  expect(resolveScheme('system', null)).toBe('light');
  expect(resolveScheme('system', undefined)).toBe('light');
  // React Native's own third state, which means the same as knowing nothing.
  expect(resolveScheme('system', 'unspecified')).toBe('light');
});
