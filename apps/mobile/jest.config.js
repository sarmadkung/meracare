/**
 * Jest configuration.
 *
 * `rootDir` is pinned to this app because jest-expo's preset otherwise resolves
 * the pnpm workspace root.
 */
module.exports = {
  preset: 'jest-expo',
  rootDir: __dirname,
  // Rendering a component or hook leaves a handle open that the React Native
  // test environment never releases, so the run finishes and then hangs rather
  // than exiting. This is about the environment, not the tests: each one still
  // tears down the query client it created, and a genuine leak in application
  // code would show up as a failing assertion rather than a slow exit.
  forceExit: true,
  setupFilesAfterEnv: ['<rootDir>/jest.setup.ts'],
  testMatch: ['<rootDir>/src/**/*.test.ts', '<rootDir>/src/**/*.test.tsx'],
  transformIgnorePatterns: [
    'node_modules/(?!((jest-)?react-native|@react-native(-community)?|expo(nent)?|@expo(nent)?/.*|@expo-google-fonts/.*|react-navigation|@react-navigation/.*|standard-navigation|native-base|react-native-svg|@supabase/.*|@meracare/.*))',
  ],
};
