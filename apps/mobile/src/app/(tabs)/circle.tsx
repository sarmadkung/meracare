import type { Senior } from '@meracare/contracts';
import { Stack, router } from 'expo-router';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, EmptyState, Screen, SummaryCard, Text } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useSeniors } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * Circle (docs/13-mvp-screen-map.md, screen 10).
 *
 * Everyone you care for, in one place. A professional caregiver with six
 * clients gets a list they can work from, which leaves Today free to be their
 * own round rather than six stacked dashboards — the same screen set serving
 * both, as docs/13 requires.
 */
export default function CircleScreen() {
  const theme = useTheme();
  const { isSignedIn } = useSession();
  const seniors = useSeniors(isSignedIn);
  const people = seniors.data ?? [];

  return (
    <Screen scrollable variant="list">
      <Stack.Screen options={{ headerShown: false }} />

      {/*
        The tab bar hides its header, so the screen has to name itself. Without
        this the list began at the top of the screen with nothing saying what
        the list was.
      */}
      <View style={{ gap: theme.spacing.xs, marginBottom: theme.spacing.sm }}>
        {people.length > 0 ? (
          <Text variant="label" color="secondary">
            {people.length === 1 ? '1 PERSON' : `${people.length} PEOPLE`}
          </Text>
        ) : null}
        <Text accessibilityRole="header" variant="pageHeading">
          Your circle
        </Text>
      </View>

      {seniors.isPending ? (
        <View
          style={{
            alignItems: 'center',
            gap: theme.spacing.md,
            paddingVertical: theme.spacing.xxl,
          }}
        >
          <ActivityIndicator color={theme.colors.primary} />
          <Text variant="secondary" color="secondary">
            Loading your care circle…
          </Text>
        </View>
      ) : seniors.isError ? (
        <Card>
          <Text variant="bodyStrong">We could not load your circle</Text>
          <Text variant="secondary" color="secondary">
            {seniors.error instanceof ApiError
              ? seniors.error.message
              : 'Something went wrong. Please try again.'}
          </Text>
          <Button variant="secondary" label="Try again" onPress={() => void seniors.refetch()} />
        </Card>
      ) : seniors.data.length === 0 ? (
        <EmptyState
          illustration="careCircle"
          title="Nobody here yet"
          body="Add the person you are caring for — or join a circle you have been invited to."
        />
      ) : (
        seniors.data.map((senior) => (
          <SummaryCard
            key={senior.id}
            name={senior.displayName}
            role={describeRole(senior)}
            href={{ pathname: '/seniors/[seniorId]', params: { seniorId: senior.id } }}
            // Per-person counts need an endpoint that batches them; fetching
            // each separately would be one request per row. Until then a card
            // without a stat strip, which SummaryCard already handles.
            stats={[]}
          />
        ))
      )}

      <View style={{ gap: theme.spacing.md, marginTop: theme.spacing.lg }}>
        <Button
          variant="secondary"
          label="Add another person"
          onPress={() => router.push('/onboarding')}
        />
        <Button
          variant="ghost"
          label="Join a care circle"
          onPress={() => router.push('/invitations/join')}
        />
      </View>
    </Screen>
  );
}

/** Plain-language description of the reader's relationship to this senior. */
function describeRole(senior: Senior): string {
  if (senior.isSelf) return 'Your own care';

  switch (senior.role) {
    case 'family_member':
      return 'Family care';
    case 'professional_caregiver':
      return 'In your professional care';
    default:
      return 'Care circle';
  }
}
