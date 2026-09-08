import { Alert, Platform } from 'react-native';

interface ConfirmActionOptions {
  title: string;
  message: string;
  confirmLabel: string;
  onConfirm: () => void | Promise<void>;
}

interface MessageOptions {
  title: string;
  message: string;
  onDismiss?: () => void;
}

/**
 * Shows a destructive confirmation on every supported platform.
 *
 * react-native-web intentionally implements Alert.alert as a no-op. Browser
 * confirm is used there so pressing a destructive button actually reaches its
 * callback; iOS and Android keep the native alert experience.
 */
export function confirmAction({
  title,
  message,
  confirmLabel,
  onConfirm,
}: ConfirmActionOptions): void {
  if (Platform.OS === 'web') {
    if (globalThis.confirm(`${title}\n\n${message}`)) {
      void onConfirm();
    }
    return;
  }

  Alert.alert(title, message, [
    { text: 'Cancel', style: 'cancel' },
    {
      text: confirmLabel,
      style: 'destructive',
      onPress: () => void onConfirm(),
    },
  ]);
}

/** Shows an acknowledgement and reliably runs its dismissal callback. */
export function showMessage({ title, message, onDismiss }: MessageOptions): void {
  if (Platform.OS === 'web') {
    globalThis.alert(`${title}\n\n${message}`);
    onDismiss?.();
    return;
  }

  Alert.alert(title, message, [{ text: 'OK', onPress: onDismiss }]);
}
