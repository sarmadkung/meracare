import { Redirect, router, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { View } from 'react-native';

import { AppleButton, Button, Card, GoogleButton, Screen, Text, TextField } from '@/components/ui';
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
  // Set when sign-in was reached from somewhere that wants the person back
  // afterwards — an invitation, most of all, which is otherwise lost the moment
  // they leave it to create an account.
  const { next } = useLocalSearchParams<{ next?: string }>();
  const {
    signIn,
    signUp,
    signInWithApple,
    signInWithGoogle,
    pending,
    isSubmitting,
    error,
    clearError,
  } = useAuthActions();

  const [mode, setMode] = useState<'signIn' | 'signUp'>('signIn');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [notice, setNotice] = useState<string | null>(null);

  if (isSignedIn) {
    return <Redirect href={destinationAfterSignIn(next)} />;
  }

  const canSubmit = email.trim().length > 0 && password.length > 0 && !isSubmitting;

  async function handleGoogle() {
    setNotice(null);
    await signInWithGoogle();
  }

  async function handleApple() {
    setNotice(null);
    await signInWithApple();
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
        <Text variant="pageHeading">GenxCare</Text>
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

        <Button
          variant="ghost"
          label="Have an invitation code?"
          onPress={() => router.push('/invitations/join')}
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
        <AppleButton onPress={handleApple} loading={pending === 'apple'} disabled={isSubmitting} />
      </View>
    </Screen>
  );
}

/**
 * Where to land once a session exists.
 *
 * Only an in-app path is honoured. `next` arrives from the URL, so anything
 * carrying a scheme or host is refused rather than followed — signing in must
 * not be a way to send somebody somewhere else.
 */
function destinationAfterSignIn(next: string | undefined) {
  const safe = next !== undefined && next.startsWith('/') && !next.startsWith('//');
  return (safe ? next : '/home') as Parameters<typeof Redirect>[0]['href'];
}
