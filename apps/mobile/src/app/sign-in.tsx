import { Redirect } from 'expo-router';
import { useState } from 'react';
import { View } from 'react-native';

import { Button, Card, GoogleButton, Screen, Text, TextField } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useAuthActions } from '@/features/auth/use-auth-actions';
import { useTheme } from '@/theme';

/**
 * Email sign-in and sign-up.
 *
 * Email and Google both end in the same Supabase session, so everything past
 * this screen is provider-agnostic (plans/phase10.md §26). The full
 * welcome/onboarding flow, and Apple sign-in, arrive with the screen map in
 * docs/13-mvp-screen-map.md.
 */
export default function SignInScreen() {
  const theme = useTheme();
  const { isSignedIn } = useSession();
  const { signIn, signUp, signInWithGoogle, pending, isSubmitting, error, clearError } =
    useAuthActions();

  const [mode, setMode] = useState<'signIn' | 'signUp'>('signIn');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [notice, setNotice] = useState<string | null>(null);

  if (isSignedIn) {
    return <Redirect href="/home" />;
  }

  const canSubmit = email.trim().length > 0 && password.length > 0 && !isSubmitting;

  async function handleGoogle() {
    setNotice(null);
    await signInWithGoogle();
  }

  async function handleSubmit() {
    setNotice(null);
    if (mode === 'signIn') {
      await signIn(email, password);
      return;
    }
    const created = await signUp(email, password);
    if (created) {
      setNotice('Check your email to confirm your account, then sign in.');
    }
  }

  function switchMode() {
    setMode((current) => (current === 'signIn' ? 'signUp' : 'signIn'));
    clearError();
    setNotice(null);
  }

  return (
    <Screen scrollable>
      <View style={{ gap: theme.spacing.sm }}>
        <Text variant="pageHeading">MeraCare</Text>
        <Text variant="body" color="secondary">
          Care for yourself, or coordinate care with your family and caregivers.
        </Text>
      </View>

      <Card>
        <Text variant="sectionHeading">
          {mode === 'signIn' ? 'Sign in' : 'Create your account'}
        </Text>

        <TextField
          label="Email"
          value={email}
          onChangeText={setEmail}
          autoCapitalize="none"
          autoComplete="email"
          keyboardType="email-address"
          inputMode="email"
          textContentType="emailAddress"
          placeholder="you@example.com"
        />

        <TextField
          label="Password"
          value={password}
          onChangeText={setPassword}
          secureTextEntry
          autoCapitalize="none"
          autoComplete={mode === 'signIn' ? 'current-password' : 'new-password'}
          textContentType={mode === 'signIn' ? 'password' : 'newPassword'}
        />

        {error ? (
          <Text variant="secondary" color="danger">
            {error}
          </Text>
        ) : null}
        {notice ? (
          <Text variant="secondary" color="success">
            {notice}
          </Text>
        ) : null}

        <Button
          label={mode === 'signIn' ? 'Sign in' : 'Create account'}
          onPress={handleSubmit}
          disabled={!canSubmit}
          loading={pending === 'email'}
        />

        <Button
          variant="ghost"
          label={mode === 'signIn' ? 'New here? Create an account' : 'I already have an account'}
          onPress={switchMode}
          disabled={isSubmitting}
        />
      </Card>

      <View style={{ gap: theme.spacing.md }}>
        <Text variant="secondary" color="secondary" style={{ textAlign: 'center' }}>
          or
        </Text>

        <GoogleButton
          onPress={handleGoogle}
          loading={pending === 'google'}
          disabled={isSubmitting}
        />
      </View>
    </Screen>
  );
}
