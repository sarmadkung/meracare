import { can } from '@genxcare/contracts';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, Screen, Text } from '@/components/ui';
import { useMedications } from '@/features/medications/use-medications';
import { useSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * Every medication a senior is on, and every one they were.
 *
 * Separate from the dose list because they answer different questions: "what do
 * I give them now" and "what are they taking". Stopped medicines stay here, at
 * the bottom, because their history is still care history
 * (plans/phase5.md §13).
 */
export default function MedicationListScreen() {
  const theme = useTheme();
  const { seniorId } = useLocalSearchParams<{ seniorId: string }>();

  const senior = useSenior(seniorId ?? null);
  const medications = useMedications(seniorId ?? null);

  const canManage = senior.data ? can(senior.data, 'medications.manage') : false;

  if (medications.isPending) {
    return (
      <Screen>
        <View style={{ alignItems: 'center', flex: 1, justifyContent: 'center' }}>
          <ActivityIndicator color={theme.colors.primary} />
        </View>
      </Screen>
    );
  }

  if (medications.isError) {
    return (
      <Screen scrollable>
        <Stack.Screen options={{ headerShown: true, title: 'Medications' }} />
        <Card>
          <Text variant="sectionHeading">We could not load these medications</Text>
          <Text variant="body" color="secondary">
            {medications.error instanceof ApiError
              ? medications.error.message
              : 'Please try again.'}
          </Text>
          <Button
            variant="secondary"
            label="Try again"
            onPress={() => void medications.refetch()}
          />
        </Card>
      </Screen>
    );
  }

  const active = medications.data.filter((medication) => medication.active);
  const stopped = medications.data.filter((medication) => !medication.active);

  return (
    <Screen scrollable>
      <Stack.Screen options={{ headerShown: true, title: 'Medications' }} />

      <Text variant="pageHeading">Medications</Text>

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

      {medications.data.length === 0 ? (
        <Card>
          <Text variant="sectionHeading">No medication yet</Text>
          <Text variant="body" color="secondary">
            Add a medicine to keep track of when it is taken.
          </Text>
        </Card>
      ) : null}

      {active.map((medication) => (
        <Card key={medication.id}>
          <Text variant="sectionHeading">{medication.name}</Text>
          {medication.dosage ? (
            <Text variant="body" color="secondary">
              {medication.dosage}
            </Text>
          ) : null}
          <Button
            variant="secondary"
            label="View"
            onPress={() =>
              router.push({
                pathname: '/medications/[medicationId]',
                params: { medicationId: medication.id },
              })
            }
          />
        </Card>
      ))}

      {stopped.length > 0 ? (
        <View style={{ gap: theme.spacing.md }}>
          <Text variant="sectionHeading">No longer taken</Text>
          {stopped.map((medication) => (
            <Card key={medication.id}>
              <Text variant="bodyStrong">{medication.name}</Text>
              <Text variant="secondary" color="secondary">
                Stopped. Its record is kept.
              </Text>
              <Button
                variant="ghost"
                label="View"
                onPress={() =>
                  router.push({
                    pathname: '/medications/[medicationId]',
                    params: { medicationId: medication.id },
                  })
                }
              />
            </Card>
          ))}
        </View>
      ) : null}
    </Screen>
  );
}
