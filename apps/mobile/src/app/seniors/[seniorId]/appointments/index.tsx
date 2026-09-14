import type { Appointment } from '@genxcare/contracts';
import { appointmentDateLabel, can } from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useCallback, useState } from 'react';
import { ActivityIndicator, FlatList, View } from 'react-native';

import { AppointmentCard, Button, Card, Screen, Text } from '@/components/ui';
import { useAppointmentHistory, useAppointments } from '@/features/appointments/use-appointments';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * Appointments for one senior.
 *
 * Upcoming comes first, because the question somebody opens this screen with is
 * "where do we have to be next?" (plans/phase6.md §§5, 16). The list is
 * virtualised and the past is paged, so a circle with years of appointments
 * behind it still opens instantly (plans/phase6.md §32).
 */

type View_ = 'upcoming' | 'today' | 'past';

const VIEWS: { view: View_; label: string }[] = [
  { view: 'upcoming', label: 'Coming up' },
  { view: 'today', label: 'Today' },
  { view: 'past', label: 'Past' },
];

export default function SeniorAppointmentsScreen() {
  const theme = useTheme();
  const { seniorId } = useLocalSearchParams<{ seniorId: string }>();

  // A view choice is transient screen state, so it lives here rather than in a
  // store, and never holds appointment data (plans/phase6.md §22).
  const [view, setView] = useState<View_>('upcoming');

  const senior = useSenior(seniorId ?? null);
  const list = useAppointments(seniorId ?? null, view === 'past' ? 'upcoming' : view);
  const history = useAppointmentHistory(view === 'past' ? (seniorId ?? null) : null);

  const timezone = senior.data?.timezone ?? 'UTC';
  const canManage = senior.data ? can(senior.data, 'appointments.manage') : false;

  const past = view === 'past';
  const appointments = past
    ? (history.data?.pages.flatMap((page) => page.items) ?? [])
    : (list.data ?? []);

  const pending = past ? history.isPending : list.isPending;
  const failed = past ? history.isError : list.isError;
  const failure = past ? history.error : list.error;

  const renderAppointment = useCallback(
    ({ item }: { item: Appointment }) => (
      <View style={{ gap: theme.spacing.xs }}>
        <Text variant="secondary" color="secondary">
          {appointmentDateLabel(item, timezone)}
        </Text>
        <AppointmentCard
          appointment={item}
          timezone={timezone}
          onPress={() =>
            router.push({
              pathname: '/appointments/[appointmentId]',
              params: { appointmentId: item.id },
            })
          }
        />
      </View>
    ),
    [theme.spacing.xs, timezone],
  );

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
      <Stack.Screen options={{ headerShown: true, title: 'Appointments' }} />

      <FlatList
        data={appointments}
        keyExtractor={(appointment) => appointment.id}
        renderItem={renderAppointment}
        contentContainerStyle={{ gap: theme.spacing.md, paddingBottom: theme.spacing.xl }}
        ListHeaderComponent={
          <View style={{ gap: theme.spacing.lg, paddingBottom: theme.spacing.md }}>
            <Text variant="pageHeading">{headingFor(view)}</Text>

            <View
              accessibilityRole="tablist"
              style={{ flexDirection: 'row', gap: theme.spacing.sm }}
            >
              {VIEWS.map((option) => (
                <Button
                  key={option.view}
                  variant={option.view === view ? 'primary' : 'secondary'}
                  label={option.label}
                  accessibilityRole="tab"
                  accessibilityState={{ selected: option.view === view }}
                  style={{ flex: 1 }}
                  onPress={() => setView(option.view)}
                />
              ))}
            </View>

            {canManage ? (
              <Button
                variant="secondary"
                label="Add an appointment"
                onPress={() =>
                  router.push({
                    pathname: '/seniors/[seniorId]/appointments/new',
                    params: { seniorId: seniorId ?? '' },
                  })
                }
              />
            ) : null}
          </View>
        }
        ListEmptyComponent={
          pending ? (
            <View style={{ alignItems: 'center', padding: theme.spacing.xl }}>
              <ActivityIndicator color={theme.colors.primary} />
            </View>
          ) : failed ? (
            <Card>
              <Text variant="sectionHeading">We could not load these appointments</Text>
              <Text variant="body" color="secondary">
                {errorMessage(failure)}
              </Text>
              <Button
                variant="secondary"
                label="Try again"
                onPress={() => void (past ? history.refetch() : list.refetch())}
              />
            </Card>
          ) : (
            <Card>
              <Text variant="sectionHeading">{emptyHeading(view)}</Text>
              <Text variant="body" color="secondary">
                {emptyBody(view)}
              </Text>
            </Card>
          )
        }
        ListFooterComponent={
          past && history.hasNextPage ? (
            <Button
              variant="secondary"
              label="Show more"
              loading={history.isFetchingNextPage}
              onPress={() => void history.fetchNextPage()}
            />
          ) : null
        }
        refreshing={past ? history.isRefetching : list.isRefetching}
        onRefresh={() => void (past ? history.refetch() : list.refetch())}
      />
    </Screen>
  );
}

function headingFor(view: View_): string {
  if (view === 'past') return 'Past appointments';
  if (view === 'today') return 'Today';
  return 'Coming up';
}

function emptyHeading(view: View_): string {
  if (view === 'past') return 'Nothing in the past yet';
  if (view === 'today') return 'Nothing today';
  return 'Nothing booked yet';
}

function emptyBody(view: View_): string {
  if (view === 'past') return 'Appointments that have already happened will appear here.';
  if (view === 'today') return 'There is nothing to be anywhere for today.';
  return 'Add an appointment to see it here.';
}

function errorMessage(error: unknown): string {
  return error instanceof ApiError ? error.message : 'Please try again.';
}
