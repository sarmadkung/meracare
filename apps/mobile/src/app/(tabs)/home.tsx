import type { Href } from 'expo-router';
import { Redirect, Stack } from 'expo-router';
import { View } from 'react-native';

import { EmptyState, Icon, ListRow, Screen, SectionHeader, Text } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useOfflineSync } from '@/features/sync/use-sync';
import { splitByAssignment, type TodayRow } from '@/features/today/today-rows';
import { useToday } from '@/features/today/use-today';
import { useTheme } from '@/theme';

/**
 * Today (docs/13-mvp-screen-map.md, screen 9).
 *
 * Everything due today, across every circle the reader belongs to.
 *
 * It used to read only tasks assigned to the reader, which is the right list
 * for a professional working a round and the wrong one for everybody else: a
 * family member caring alone assigns nothing to themselves, so the screen said
 * "Nothing needs you" while their mother had four doses due. Assignment now
 * orders the screen instead of filtering it.
 */
export default function HomeScreen() {
  const theme = useTheme();
  const { isSignedIn, isRestoring } = useSession();
  const today = useToday(isSignedIn);

  // Anything recorded while offline is sent as soon as the app is usable.
  useOfflineSync();

  if (!isRestoring && !isSignedIn) {
    return <Redirect href="/sign-in" />;
  }

  const { mine, rest } = splitByAssignment(today.data ?? []);

  return (
    <Screen scrollable variant="list">
      <Stack.Screen options={{ headerShown: false }} />

      <View style={{ gap: theme.spacing.xs, marginBottom: theme.spacing.sm }}>
        <Text variant="label" color="secondary">
          {new Date().toLocaleDateString(undefined, {
            weekday: 'long',
            day: 'numeric',
            month: 'long',
          })}
        </Text>
        <Text accessibilityRole="header" variant="pageHeading">
          Today
        </Text>
      </View>

      {mine.length === 0 && rest.length === 0 ? (
        <EmptyState
          illustration="allCaughtUp"
          title="Nothing needs you"
          body="When something is due for anyone in your circle, it will appear here."
        />
      ) : (
        <>
          {/*
            Only when there is something to separate. A heading over a single
            list just makes the reader wonder what the other section is.
          */}
          {mine.length > 0 ? (
            <>
              <SectionHeader title="Yours to do" />
              {mine.map((row) => (
                <Row key={row.id} row={row} />
              ))}
            </>
          ) : null}

          {rest.length > 0 ? (
            <>
              {mine.length > 0 ? <SectionHeader title="Also today" /> : null}
              {rest.map((row) => (
                <Row key={row.id} row={row} />
              ))}
            </>
          ) : null}
        </>
      )}
    </Screen>
  );
}

/** One thing happening today, with the icon for the kind of care it is. */
function Row({ row }: { row: TodayRow }) {
  const theme = useTheme();

  return (
    <ListRow
      title={row.title}
      subtitle={row.subtitle}
      href={row.href as Href}
      leading={
        <View
          style={{
            alignItems: 'center',
            backgroundColor: theme.colors.primarySubtle,
            borderRadius: theme.radii.md,
            height: 40,
            justifyContent: 'center',
            width: 40,
          }}
        >
          <Icon name={row.icon} color={theme.colors.primary} />
        </View>
      }
    />
  );
}
