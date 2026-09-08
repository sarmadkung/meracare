import type { ApiErrorBody, SeniorCreateMode } from '@genxcare/contracts';
import { router } from 'expo-router';
import { useState } from 'react';
import { View } from 'react-native';

import { Button, Card, Illustration, OptionCard, Screen, Text, TextField } from '@/components/ui';
import { useCreateSenior } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useUIStore } from '@/stores/ui-store';
import { useTheme } from '@/theme';

/**
 * Onboarding: choose how you will use GenXcare, then create the first profile.
 *
 * The three choices are the product's care modes (docs/01). They set the
 * creator's role in the new care circle — they do not select a different app.
 */
const MODES: { mode: SeniorCreateMode; title: string; description: string; nameLabel: string }[] = [
  {
    mode: 'self',
    title: 'For myself',
    description: 'Track your own medications, tasks and appointments. You can invite people later.',
    nameLabel: 'Your name',
  },
  {
    mode: 'family_member',
    title: 'For a family member',
    description: 'Coordinate care for a parent or relative, together with family and caregivers.',
    nameLabel: "Your family member's name",
  },
  {
    mode: 'professional_caregiver',
    title: "I'm a professional caregiver",
    description: 'Look after the people in your care. You can add more than one.',
    nameLabel: "The person's name",
  },
];

export default function OnboardingScreen() {
  const theme = useTheme();
  const createSenior = useCreateSenior();
  const setSelectedSeniorId = useUIStore((state) => state.setSelectedSeniorId);

  const [mode, setMode] = useState<SeniorCreateMode>('self');
  const [displayName, setDisplayName] = useState('');

  const selected = MODES.find((option) => option.mode === mode) ?? MODES[0]!;
  const fieldErrors = validationDetails(createSenior.error);

  async function handleSubmit() {
    const created = await createSenior.mutateAsync({
      mode,
      displayName: displayName.trim(),
      // The device's timezone is the best first guess at the senior's own, and
      // it decides when their day starts. It is editable on the profile
      // afterwards, for a senior who lives somewhere else.
      timezone: deviceTimezone(),
    });
    setSelectedSeniorId(created.id);
    router.replace('/home');
  }

  return (
    <Screen scrollable>
      <Illustration name="welcome" height={180} />

      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">Welcome to GenXcare</Text>
        <Text variant="body" color="secondary">
          How would you like to start? You can change this later, and you never have to invite
          anyone.
        </Text>
      </View>

      <View accessibilityRole="radiogroup" style={{ gap: theme.spacing.md }}>
        {MODES.map((option) => (
          <OptionCard
            key={option.mode}
            title={option.title}
            description={option.description}
            selected={option.mode === mode}
            onPress={() => setMode(option.mode)}
          />
        ))}
      </View>

      <Card>
        <TextField
          label={selected.nameLabel}
          value={displayName}
          onChangeText={setDisplayName}
          autoCapitalize="words"
          autoComplete="name"
          placeholder="e.g. Ahmed Khan"
          error={fieldErrors.displayName}
        />

        {createSenior.isError && Object.keys(fieldErrors).length === 0 ? (
          <Text variant="secondary" color="danger">
            {createSenior.error instanceof ApiError
              ? createSenior.error.message
              : 'We could not save that just now. Please try again.'}
          </Text>
        ) : null}

        <Button
          label="Continue"
          onPress={handleSubmit}
          disabled={displayName.trim().length === 0}
          loading={createSenior.isPending}
        />
      </Card>
    </Screen>
  );
}

/**
 * The device's IANA timezone.
 *
 * Falls back to UTC on a platform that cannot report one, which matches the
 * column default rather than guessing.
 */
function deviceTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

/** Field-level messages from a VALIDATION_FAILED response, if any. */
function validationDetails(error: unknown): Record<string, string> {
  if (error instanceof ApiError && error.details) {
    return error.details;
  }
  return {} as NonNullable<ApiErrorBody['error']['details']>;
}
