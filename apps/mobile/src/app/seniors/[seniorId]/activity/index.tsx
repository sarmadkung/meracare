import type { CareEvent } from '@genxcare/contracts';
import { groupByDay } from '@genxcare/contracts';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useMemo } from 'react';
import { ActivityIndicator, SectionList, View } from 'react-native';

import { ActivityRow, Button, Card, Screen, Text } from '@/components/ui';
import { useActivity } from '@/features/activity/use-activity';
import { useCircleMembers } from '@/features/circle/use-circle';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * The activity timeline for one senior.
 *
 * One feed across every domain, because the question somebody opens this with
 * is "what has been happening?" — not "what happened to the tasks"
 * (plans/phase7.md, objective).
 *
 * A SectionList rather than a FlatList of pre-grouped rows: the day headings
 * stick as the list scrolls, which is what keeps "10:42" meaningful thirty rows
 * down, and the rows are still virtualised for a timeline that has no end
 * (plans/phase7.md §§16, 30).
 */
export default function SeniorActivityScreen() {
  const theme = useTheme();
  const { seniorId } = useLocalSearchParams<{ seniorId: string }>();

  const senior = useSenior(seniorId ?? null);
  const members = useCircleMembers(seniorId ?? null);
  const activity = useActivity(seniorId ?? null);

  const timezone = senior.data?.timezone ?? 'UTC';

  const events = useMemo(
    () => activity.data?.pages.flatMap((page) => page.items) ?? [],
    [activity.data],
  );

  // Grouped in the senior's timezone, so an event at half past midnight in
  // Karachi is filed under that day there (plans/phase7.md §16).
  const sections = useMemo(
    () =>
      groupByDay(events, timezone).map((day) => ({
        key: day.key,
        title: day.heading,
        data: day.events,
      })),
    [events, timezone],
  );

  // One lookup for the whole list rather than a search per row, so a long
  // timeline does not get slower as it grows (plans/phase7.md §30).
  const names = useMemo(() => {
    const byUserId = new Map<string, string>();
    for (const member of members.data ?? []) {
      byUserId.set(member.userId, member.displayName);
    }
    return byUserId;
  }, [members.data]);

  // "Somebody" covers two real cases: an event with no human actor, and one
  // whose actor has since left the circle and is no longer in the member list.
  const actorName = (event: CareEvent) =>
    (event.actorUserId === null ? undefined : names.get(event.actorUserId)) ?? 'Somebody';

  if (senior.isPending) {
    return (
      <Screen>
        <View style={{ alignItems: 'center', flex: 1, justifyContent: 'center' }}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      </Screen>
    );
  }

  return (
    <Screen>
      <Stack.Screen options={{ headerShown: true, title: 'Activity' }} />

      <SectionList
        sections={sections}
        keyExtractor={(event) => event.id}
        stickySectionHeadersEnabled
        renderItem={({ item }) => (
          <ActivityRow event={item} timezone={timezone} actorName={actorName(item)} />
        )}
        renderSectionHeader={({ section }) => (
          <View
            style={{
              backgroundColor: theme.colors.background,
              paddingBottom: theme.spacing.sm,
              paddingTop: theme.spacing.md,
            }}
          >
            <Text variant="sectionHeading">{section.title}</Text>
          </View>
        )}
        contentContainerStyle={{ gap: theme.spacing.md, paddingBottom: theme.spacing.xl }}
        ListHeaderComponent={
          <View style={{ gap: theme.spacing.sm, paddingBottom: theme.spacing.sm }}>
            <Text variant="pageHeading">Activity</Text>
            <Text variant="body" color="secondary">
              What you and the care circle have been doing.
            </Text>
          </View>
        }
        ListEmptyComponent={
          activity.isPending ? (
            <View style={{ alignItems: 'center', padding: theme.spacing.xl }}>
              <ActivityIndicator color={theme.colors.primary} />
            </View>
          ) : activity.isError ? (
            <Card>
              <Text variant="sectionHeading">We could not load this activity</Text>
              <Text variant="body" color="secondary">
                {activity.error instanceof ApiError ? activity.error.message : 'Please try again.'}
              </Text>
              <Button
                variant="secondary"
                label="Try again"
                onPress={() => void activity.refetch()}
              />
            </Card>
          ) : (
            <Card>
              <Text variant="sectionHeading">No activity yet</Text>
              <Text variant="body" color="secondary">
                Care activity will appear here as you and your care circle work together.
              </Text>
            </Card>
          )
        }
        ListFooterComponent={
          activity.hasNextPage ? (
            <Button
              variant="secondary"
              label="Show more"
              loading={activity.isFetchingNextPage}
              onPress={() => void activity.fetchNextPage()}
            />
          ) : null
        }
        refreshing={activity.isRefetching}
        onRefresh={() => void activity.refetch()}
      />
    </Screen>
  );
}
