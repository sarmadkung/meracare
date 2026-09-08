import { Stack } from 'expo-router';

/**
 * Settings is a stack so Profile and Notification settings push over its index
 * with a back button, instead of replacing the tab wholesale.
 *
 * Header styling comes from the root layout; screens here declare only a title.
 */
export default function SettingsLayout() {
  return <Stack screenOptions={{ headerShown: true }} />;
}
