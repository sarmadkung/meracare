import { configure } from '@testing-library/react-native';

/**
 * Jest setup.
 *
 * Environment variables are set here because `src/lib/env.ts` throws when they
 * are missing — that check is deliberate, so tests supply values rather than
 * weaken it.
 */
process.env.EXPO_PUBLIC_SUPABASE_URL ??= 'https://test.supabase.co';
process.env.EXPO_PUBLIC_SUPABASE_ANON_KEY ??= 'test-anon-key';
process.env.EXPO_PUBLIC_API_URL ??= 'http://localhost:8080';

/**
 * `@expo/vector-icons` loads its glyph font asynchronously, which resolves
 * after the test's render has finished and produces an "update was not wrapped
 * in act(...)" warning on every screen that draws an icon. The font is already
 * bundled and there is nothing to wait for in a test environment, so report it
 * as loaded and keep the output clean enough that a real warning is visible.
 */
jest.mock('expo-font', () => ({
  ...jest.requireActual('expo-font'),
  useFonts: () => [true, null],
  isLoaded: () => true,
  loadAsync: jest.fn(() => Promise.resolve()),
}));

/**
 * Give async queries room on a loaded machine.
 *
 * The default ceiling is one second. A screen like the senior dashboard fires
 * six queries in parallel, and when the whole suite runs at once that can take
 * longer than a second to settle — so the test failed by the clock rather than
 * for anything it was written to catch. This was already flaky before the
 * design work and got worse as the suite grew.
 *
 * waitFor still returns the moment its condition holds, so a higher ceiling
 * costs nothing on a passing test.
 */
configure({ asyncUtilTimeout: 5000 });
