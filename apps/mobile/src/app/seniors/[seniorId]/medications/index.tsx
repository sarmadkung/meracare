import type { MedicationDose, MedicationDoseScope } from '@genxcare/contracts';
import { can } from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useCallback, useState } from 'react';
import { ActivityIndicator, FlatList, View } from 'react-native';

import { Button, Card, MedicationCard, Screen, Text } from '@/components/ui';
import {
  useMedicationDoses,
  useSkipDose,
  useTakeDose,
} from '@/features/medications/use-medications';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * Medication for one senior.
 *
 * Today comes first because it is what somebody standing with the person needs
 * (plans/phase5.md §14). The list is virtualised: a long-running circle
 * accumulates a lot of doses, and only what is on screen should be rendered.
 */

const VIEWS: { scope: MedicationDoseScope; label: string }[] = [
  { scope: 'today', label: 'Today' },
  { scope: 'upcoming', label: 'Coming up' },
  { scope: 'missed', label: 'Missed' },
];

export default function SeniorMedicationsScreen() {
  const theme = useTheme();
  const { seniorId } = useLocalSearchParams<{ seniorId: string }>();

  // A view choice is transient screen state, so it lives here rather than in a
  // store, and never holds medication data (plans/phase5.md §19).
  const [scope, setScope] = useState<MedicationDoseScope>('today');

  const senior = useSenior(seniorId ?? null);
  const doses = useMedicationDoses(seniorId ?? null, scope);
  const take = useTakeDose(seniorId ?? '');
  const skip = useSkipDose(seniorId ?? '');

  const timezone = senior.data?.timezone ?? 'UTC';
  const canRecord = senior.data ? can(senior.data, 'medications.record') : false;
  const canManage = senior.data ? can(senior.data, 'medications.manage') : false;

  const renderDose = useCallback(
    ({ item }: { item: MedicationDose }) => (
      <MedicationCard
        dose={item}
        timezone={timezone}
        canRecord={canRecord}
        onPress={() =>
          router.push({
            pathname: '/medications/[medicationId]',
            params: { medicationId: item.medicationId },
          })
        }
        onTake={() => take.mutate({ medicationId: item.medicationId, doseId: item.id })}
        onSkip={() => skip.mutate({ medicationId: item.medicationId, doseId: item.id })}
      />
    ),
    [timezone, canRecord, take, skip],
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
      <Stack.Screen options={{ headerShown: true, title: 'Medication' }} />

      <FlatList
        data={doses.data ?? []}
        keyExtractor={(dose) => dose.id}
        renderItem={renderDose}
        contentContainerStyle={{ gap: theme.spacing.md, paddingBottom: theme.spacing.xl }}
        ListHeaderComponent={
          <View style={{ gap: theme.spacing.lg, paddingBottom: theme.spacing.md }}>
            <Text variant="pageHeading">{headingFor(scope)}</Text>

            <View
              accessibilityRole="tablist"
              style={{ flexDirection: 'row', gap: theme.spacing.sm }}
            >
              {VIEWS.map((view) => (
                <Button
                  key={view.scope}
                  variant={view.scope === scope ? 'primary' : 'secondary'}
                  label={view.label}
                  accessibilityRole="tab"
                  accessibilityState={{ selected: view.scope === scope }}
                  style={{ flex: 1 }}
                  onPress={() => setScope(view.scope)}
                />
              ))}
            </View>

            <Button
              variant="secondary"
              label="All medications"
              onPress={() =>
                router.push({
                  pathname: '/seniors/[seniorId]/medications/list',
                  params: { seniorId: seniorId ?? '' },
                })
              }
            />

            {canManage ? (
              <Button
                variant="secondary"
                label="Add a medication"
                onPress={() =>
                  router.push({
                    pathname: '/seniors/[seniorId]/medications/new',
                    params: { seniorId: seniorId ?? '' },
                  })
                }
              />
            ) : null}

            {take.isError || skip.isError ? (
              <Card>
                <Text variant="bodyStrong" color="danger">
                  That did not save
                </Text>
                <Text variant="body" color="secondary">
                  {errorMessage(take.error ?? skip.error)}
                </Text>
              </Card>
            ) : null}
          </View>
        }
        ListEmptyComponent={
          doses.isPending ? (
            <View style={{ alignItems: 'center', padding: theme.spacing.xl }}>
              <ActivityIndicator color={theme.colors.primary} />
            </View>
          ) : doses.isError ? (
            <Card>
              <Text variant="sectionHeading">We could not load this medication</Text>
              <Text variant="body" color="secondary">
                {errorMessage(doses.error)}
              </Text>
              <Button variant="secondary" label="Try again" onPress={() => void doses.refetch()} />
            </Card>
          ) : (
            <Card>
              <Text variant="sectionHeading">{emptyHeading(scope)}</Text>
              <Text variant="body" color="secondary">
                {emptyBody(scope)}
              </Text>
            </Card>
          )
        }
        refreshing={doses.isRefetching}
        onRefresh={() => void doses.refetch()}
      />
    </Screen>
  );
}

function headingFor(scope: MedicationDoseScope): string {
  if (scope === 'missed') return 'Missed';
  if (scope === 'upcoming') return 'Coming up';
  return "Today's medication";
}

function emptyHeading(scope: MedicationDoseScope): string {
  if (scope === 'missed') return 'Nothing has been missed';
  if (scope === 'upcoming') return 'Nothing scheduled yet';
  return 'Nothing to take today';
}

function emptyBody(scope: MedicationDoseScope): string {
  if (scope === 'missed') return 'Everything due so far has been dealt with.';
  if (scope === 'upcoming') return 'Medication added for the coming week will appear here.';
  return 'Add a medication to see it here.';
}

function errorMessage(error: unknown): string {
  return error instanceof ApiError ? error.message : 'Please try again.';
}
