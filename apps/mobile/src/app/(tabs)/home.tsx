import { useQueryClient } from '@tanstack/react-query';
import type { Href } from 'expo-router';
import { Stack, router } from 'expo-router';
import { useMemo, useState } from 'react';
import { ActivityIndicator, View } from 'react-native';

import {
  AgendaRow,
  AttentionBanner,
  Button,
  Card,
  EmptyState,
  ListRow,
  PersonFilter,
  Screen,
  Text,
} from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useSeniorSummaries } from '@/features/seniors/use-senior-summaries';
import { useSeniors } from '@/features/seniors/use-seniors';
import { useOfflineSync } from '@/features/sync/use-sync';
import { settleAgendaItem } from '@/features/today/settle';
import {
  attention,
  buildAgenda,
  daySummary,
  peopleInDay,
  type AgendaEntry,
  type DaySummary,
} from '@/features/today/today-agenda';
import { useToday } from '@/features/today/use-today';
import { useTheme } from '@/theme';

/**
 * Today (docs/13-mvp-screen-map.md, screen 9).
 *
 * One timeline across every circle, narrowed by a person filter.
 *
 * The filter is deliberately not remembered between launches. A filter you
 * forgot you set is how a dose gets missed, which is the single failure this
 * screen exists to prevent.
 */
export default function HomeScreen() {
  const theme = useTheme();
  const queryClient = useQueryClient();
  const { isSignedIn } = useSession();

  const today = useToday(isSignedIn);
  const seniors = useSeniors(isSignedIn);
  const summaries = useSeniorSummaries(isSignedIn);

  const [selected, setSelected] = useState<string | null>(null);
  const [settling, setSettling] = useState<string | null>(null);

  // Anything recorded while offline is sent as soon as the app is usable.
  useOfflineSync();

  const people = useMemo(
    () => peopleInDay(seniors.data ?? [], summaries.data ?? []),
    [seniors.data, summaries.data],
  );

  // A person can be chosen and then leave the circle; falling back to everyone
  // is better than an empty screen filtered to somebody who is not there.
  const chosen = people.find((person) => person.seniorId === selected) ?? null;

  const entries = useMemo(() => {
    const items = (today.data ?? []).filter(
      (item) => chosen === null || item.seniorId === chosen.seniorId,
    );

    return buildAgenda(items, { now: new Date(), namePeople: chosen === null });
  }, [today.data, chosen]);

  const slipped = attention(entries);

  async function settle(entry: AgendaEntry, outcome: 'done' | 'skipped') {
    if (entry.action === null) return;

    setSettling(entry.id);
    try {
      await settleAgendaItem(queryClient, entry.action, outcome);
    } catch {
      // settleAgendaItem has already put the row back; the refreshed day is the
      // message. A toast here would be a second thing to dismiss on a screen
      // whose whole job is to be glanced at.
    } finally {
      setSettling(null);
    }
  }

  return (
    <Screen scrollable variant="list">
      <Stack.Screen options={{ headerShown: false }} />

      <View style={{ gap: theme.spacing.xs }}>
        <Text variant="label" color="secondary">
          {new Date().toLocaleDateString(undefined, {
            weekday: 'long',
            day: 'numeric',
            month: 'long',
          })}
        </Text>
        <Text accessibilityRole="header" variant="pageHeading">
          {chosen === null ? 'Today' : chosen.isSelf ? 'Your day' : chosen.name}
        </Text>

        {/*
          Only once a person is chosen. Across the whole circle there is no one
          clock and no one count to give, and the city is most of the point:
          the gutter is drawn in the senior's zone, so a reader in another one
          has to be told whose 14:00 they are looking at.
        */}
        {chosen === null ? null : <DayLine summary={daySummary(entries, chosen.timezone)} />}
      </View>

      <PersonFilter people={people} selected={chosen?.seniorId ?? null} onSelect={setSelected} />

      {slipped === null ? null : (
        <AttentionBanner
          title={slipped.title}
          detail={slipped.detail}
          onPress={
            slipped.href === null ? undefined : () => router.push(slipped.href as unknown as Href)
          }
        />
      )}

      {today.isPending ? (
        <View style={{ alignItems: 'center', paddingVertical: theme.spacing.xxl }}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      ) : today.isError ? (
        <Card>
          <Text variant="bodyStrong">We could not load your day</Text>
          <Text variant="secondary" color="secondary">
            Something went wrong. Please try again.
          </Text>
          <Button variant="secondary" label="Try again" onPress={() => void today.refetch()} />
        </Card>
      ) : entries.length === 0 ? (
        chosen === null ? (
          <EmptyState
            illustration="allCaughtUp"
            title="Nothing needs you"
            body="When something is due for anyone in your circle, it will appear here."
          />
        ) : (
          // Not "nothing needs you": the day is not clear, it is clear for this
          // one person, and the rest of the circle is one tap away.
          <EmptyState
            illustration="allCaughtUp"
            title={`Nothing due for ${chosen.name}`}
            body="Their day is clear. Tap Everyone to see the rest of your circle."
          />
        )
      ) : (
        <View>
          {entries.map((entry, index) => (
            <AgendaRow
              key={entry.id}
              time={entry.time}
              showTime={entry.showTime}
              icon={entry.icon}
              title={entry.title}
              detail={entry.detail}
              tone={entry.tone}
              // Assignment marks a row rather than heading a section: a family
              // member caring alone assigns nothing to themselves, so the
              // section it used to head was permanently empty.
              status={entry.status ?? (entry.assignedToMe ? 'Yours' : null)}
              isLast={index === entries.length - 1}
              onPress={() => router.push(entry.href as Href)}
              {...actionProps(entry, settling === entry.id, settle)}
            />
          ))}
        </View>
      )}

      {/*
        A filter must never be a dead end. From one person's day there is one
        tap into the rest of their care — and it matters most when the day is
        empty, which is exactly when there is nothing else on screen to touch.
      */}
      {chosen === null ? null : (
        <ListRow
          title={`Open ${chosen.isSelf ? 'your' : `${chosen.name}’s`} care`}
          subtitle="Medications, tasks, notes and history"
          href={
            {
              pathname: '/seniors/[seniorId]',
              params: { seniorId: chosen.seniorId },
            } as Href
          }
        />
      )}
    </Screen>
  );
}

/**
 * How the chosen person's day is going, under their name.
 *
 * Drawn as separate pieces rather than one interpolated string so the count
 * that has slipped can carry the warning colour, and spoken as one sentence so
 * a screen reader does not stop four times on one line.
 */
function DayLine({ summary }: { summary: DaySummary }) {
  const theme = useTheme();

  const parts = [summary.left, summary.attention, summary.place].filter(
    (part): part is string => part !== null,
  );

  return (
    <View
      accessible
      accessibilityLabel={parts.join(', ')}
      style={{ alignItems: 'center', flexDirection: 'row', flexWrap: 'wrap', gap: 6 }}
    >
      <Text variant="secondary" color="secondary">
        {summary.left}
      </Text>

      {summary.attention === null ? null : (
        <>
          <Text variant="secondary" color="muted">
            &#183;
          </Text>
          <Text variant="secondary" style={{ color: theme.colors.warning, fontWeight: '600' }}>
            {summary.attention}
          </Text>
        </>
      )}

      {summary.place === null ? null : (
        <>
          <Text variant="secondary" color="muted">
            &#183;
          </Text>
          <Text variant="secondary" color="secondary">
            {summary.place}
          </Text>
        </>
      )}
    </View>
  );
}

/**
 * Buttons for the current stop, and nothing for the rest.
 *
 * An outcome on every row would make the day a form; here there is one obvious
 * thing to do, and everything else is a list.
 */
function actionProps(
  entry: AgendaEntry,
  settling: boolean,
  settle: (entry: AgendaEntry, outcome: 'done' | 'skipped') => void,
) {
  if (entry.action === null || entry.tone !== 'now') return {};

  return {
    settleLabel: entry.action.kind === 'dose' ? 'Mark as taken' : 'Mark as done',
    settling,
    onSettle: (outcome: 'done' | 'skipped') => settle(entry, outcome),
  };
}
