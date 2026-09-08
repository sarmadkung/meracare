import { Alert, Platform } from 'react-native';

import { confirmAction, showMessage } from '@/lib/dialogs';

const originalOS = Platform.OS;
const originalConfirm = Object.getOwnPropertyDescriptor(globalThis, 'confirm');
const originalAlert = Object.getOwnPropertyDescriptor(globalThis, 'alert');

function usePlatform(os: typeof Platform.OS) {
  Object.defineProperty(Platform, 'OS', { configurable: true, value: os });
}

function installBrowserFunction(name: 'confirm' | 'alert', value: jest.Mock) {
  Object.defineProperty(globalThis, name, { configurable: true, value, writable: true });
}

function restoreBrowserFunction(name: 'confirm' | 'alert', descriptor?: PropertyDescriptor) {
  if (descriptor) {
    Object.defineProperty(globalThis, name, descriptor);
  } else {
    Reflect.deleteProperty(globalThis, name);
  }
}

afterEach(() => {
  usePlatform(originalOS);
  restoreBrowserFunction('confirm', originalConfirm);
  restoreBrowserFunction('alert', originalAlert);
  jest.restoreAllMocks();
});

it('runs a confirmed destructive action on web', () => {
  usePlatform('web');
  const confirm = jest.fn(() => true);
  installBrowserFunction('confirm', confirm);
  const onConfirm = jest.fn();

  confirmAction({
    title: 'Remove profile?',
    message: 'This cannot be undone.',
    confirmLabel: 'Remove',
    onConfirm,
  });

  expect(confirm).toHaveBeenCalledWith('Remove profile?\n\nThis cannot be undone.');
  expect(onConfirm).toHaveBeenCalledTimes(1);
});

it('does not run a cancelled destructive action on web', () => {
  usePlatform('web');
  installBrowserFunction(
    'confirm',
    jest.fn(() => false),
  );
  const onConfirm = jest.fn();

  confirmAction({
    title: 'Remove profile?',
    message: 'This cannot be undone.',
    confirmLabel: 'Remove',
    onConfirm,
  });

  expect(onConfirm).not.toHaveBeenCalled();
});

it('uses the native alert outside web', () => {
  usePlatform('ios');
  const alert = jest.spyOn(Alert, 'alert').mockImplementation();
  const onConfirm = jest.fn();

  confirmAction({
    title: 'Remove profile?',
    message: 'This cannot be undone.',
    confirmLabel: 'Remove',
    onConfirm,
  });

  const buttons = alert.mock.calls[0]?.[2];
  buttons?.[1]?.onPress?.();
  expect(onConfirm).toHaveBeenCalledTimes(1);
});

it('dismisses a web message after showing it', () => {
  usePlatform('web');
  const alert = jest.fn();
  installBrowserFunction('alert', alert);
  const onDismiss = jest.fn();

  showMessage({ title: 'Profile deleted', message: 'Done.', onDismiss });

  expect(alert).toHaveBeenCalledWith('Profile deleted\n\nDone.');
  expect(onDismiss).toHaveBeenCalledTimes(1);
});
