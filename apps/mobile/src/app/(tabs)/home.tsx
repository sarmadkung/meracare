import { useQueryClient } from '@tanstack/react-query';
import type { Href } from 'expo-router';
import { Redirect, Stack, router } from 'expo-router';
import { useMemo, useState } from 'react';
import { ActivityIndicator, View } from 'react-native';

import {
  AgendaRow,
  AttentionBanner,
  Button,
  Card,
  EmptyState,
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
  peopleInDay,
  type AgendaEntry,
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
  const { isSignedIn, isRestoring } = useSession();

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

  if (!isRestoring && !isSignedIn) {
    return <Redirect href="/sign-in" />;
  }

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
          {chosen?.name ?? 'Today'}
        </Text>
      </View>

      <PersonFilter people={people} selected={chosen?.seniorId ?? null} onSelect={setSelected} />

      {slipped === null ? null : <AttentionBanner title={slipped.title} detail={slipped.detail} />}

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
    </Screen>
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
