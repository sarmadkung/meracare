import { NOTIFICATION_CATEGORIES } from '@meracare/contracts';
import { Stack } from 'expo-router';
import { useCallback, useEffect, useState } from 'react';
import { ActivityIndicator, Linking, StyleSheet, Switch, View } from 'react-native';

import { Button, Card, Screen, Text } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import {
  notificationPermission,
  permissionAllowsDelivery,
  requestNotificationPermission,
  type PermissionState,
} from '@/features/notifications/permission';
import {
  useNotificationPreferences,
  useRegisterDevice,
  useUpdateNotificationPreferences,
} from '@/features/notifications/use-notifications';
import { useTheme } from '@/theme';

/**
 * Notification settings (docs/13-mvp-screen-map.md, screen 25).
 *
 * Two independent things decide whether a phone makes a sound, and the screen
 * shows both: what MeraCare has been asked to send, and what the operating
 * system allows. Showing only the first is how an app ends up insisting
 * reminders are on while the user hears nothing (plans/phase8.md §6).
 */
export default function NotificationSettingsScreen() {
  const theme = useTheme();
  const { isSignedIn } = useSession();

  const preferences = useNotificationPreferences(isSignedIn);
  const update = useUpdateNotificationPreferences();
  const register = useRegisterDevice();

  const [permission, setPermission] = useState<PermissionState | null>(null);

  useEffect(() => {
    let cancelled = false;
    void notificationPermission().then((state) => {
      if (!cancelled) setPermission(state);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const askForPermission = useCallback(async () => {
    const state = await requestNotificationPermission();
    setPermission(state);

    // A token only exists once permission does, so this is the moment to tell
    // the server about this device again (plans/phase8.md §9).
    if (permissionAllowsDelivery(state)) register.mutate();
  }, [register]);

  if (preferences.isPending) {
    return (
      <Screen>
        <Stack.Screen options={{ headerShown: true, title: 'Notifications' }} />
        <View style={styles.centred}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      </Screen>
    );
  }

  if (preferences.isError) {
    return (
      <Screen scrollable>
        <Stack.Screen options={{ headerShown: true, title: 'Notifications' }} />
        <Card>
          <Text variant="sectionHeading">We could not load your settings</Text>
          <Text variant="body" color="secondary">
            Your reminders are unchanged. Please try again.
          </Text>
          <Button variant="secondary" label="Try again" onPress={() => preferences.refetch()} />
        </Card>
      </Screen>
    );
  }

  const current = preferences.data;

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Notifications' }} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">Notifications</Text>
        <Text variant="body" color="secondary">
          MeraCare reminds you before care is due and can alert you when care remains unrecorded.
          Notifications never say what the medicine is.
        </Text>
      </View>

      <PermissionCard state={permission} onAsk={askForPermission} />

      <Card>
        <Text variant="sectionHeading">What to remind me about</Text>

        {NOTIFICATION_CATEGORIES.map((category) => (
          <CategoryRow
            key={category.key}
            label={category.label}
            description={category.description}
            value={current[category.key]}
            // Every switch is disabled together while one is saving: the
            // categories share a single settings row on the server, and two
            // requests in flight could land in either order.
            disabled={update.isPending}
            onChange={(value) => update.mutate({ [category.key]: value })}
          />
        ))}

        {update.isError ? (
          <Text variant="body" style={{ color: theme.colors.danger }}>
            That change was not saved. Check your connection and try again.
          </Text>
        ) : null}
      </Card>

      <Card>
        <Text variant="sectionHeading">About reminders</Text>
        <Text variant="body" color="secondary">
          Upcoming reminders are scheduled on this device, so they arrive without a connection.
          Missed-dose and care-activity alerts come from MeraCare and need a connection.
        </Text>
      </Card>
    </Screen>
  );
}

/**
 * The operating system's permission, in the user's terms.
 *
 * Each state gets its own wording because each needs a different action from
 * the user: one is a button, one is a trip to the system settings, and one is
 * nothing at all (plans/phase8.md §19).
 */
function PermissionCard({ state, onAsk }: { state: PermissionState | null; onAsk: () => void }) {
  const theme = useTheme();

  if (state === null || state === 'granted') return null;

  if (state === 'undetermined') {
    return (
      <Card>
        <Text variant="sectionHeading">Turn on notifications</Text>
        <Text variant="body" color="secondary">
          MeraCare needs your phone&apos;s permission before it can remind you. You can change this
          at any time.
        </Text>
        {/* Asked here, in context, rather than on first launch: somebody who
            understands what the reminders are for is far likelier to say yes,
            and the prompt cannot be shown twice (plans/phase8.md §19). */}
        <Button label="Allow notifications" onPress={onAsk} />
      </Card>
    );
  }

  if (state === 'provisional') {
    return (
      <Card>
        <Text variant="sectionHeading">Reminders arrive quietly</Text>
        <Text variant="body" color="secondary">
          Your phone is delivering MeraCare reminders silently, to the notification centre. Allow
          notifications in Settings to hear them.
        </Text>
        <Button variant="secondary" label="Open Settings" onPress={() => Linking.openSettings()} />
      </Card>
    );
  }

  return (
    <Card>
      <Text variant="sectionHeading" style={{ color: theme.colors.warning }}>
        Notifications are turned off
      </Text>
      <Text variant="body" color="secondary">
        Your phone is blocking MeraCare reminders, so nothing below will reach you. You can turn
        them back on in your phone&apos;s settings.
      </Text>
      <Button variant="secondary" label="Open Settings" onPress={() => Linking.openSettings()} />
    </Card>
  );
}

/** One category switch. */
function CategoryRow({
  label,
  description,
  value,
  disabled,
  onChange,
}: {
  label: string;
  description: string;
  value: boolean;
  disabled: boolean;
  onChange: (value: boolean) => void;
}) {
  const theme = useTheme();

  return (
    <View style={[styles.row, { gap: theme.spacing.md, minHeight: theme.minTouchTarget }]}>
      <View style={styles.rowText}>
        <Text variant="body">{label}</Text>
        <Text variant="secondary" color="secondary">
          {description}
        </Text>
      </View>

      <Switch
        value={value}
        disabled={disabled}
        onValueChange={onChange}
        // The switch's own position and its platform on/off wording carry the
        // state; nothing here depends on the colour alone.
        accessibilityLabel={label}
        accessibilityHint={description}
        trackColor={{ false: theme.colors.border, true: theme.colors.primary }}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  centred: { alignItems: 'center', flex: 1, justifyContent: 'center' },
  row: { alignItems: 'center', flexDirection: 'row' },
  rowText: { flex: 1, gap: 2 },
});
