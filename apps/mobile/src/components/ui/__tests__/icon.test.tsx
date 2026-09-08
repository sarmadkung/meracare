import { render, screen } from '@testing-library/react-native';

import { ThemeProvider } from '@/theme';

import { Icon } from '../icon';

/**
 * An icon beside its own label is decoration and must stay silent, or a screen
 * reader announces everything twice. Callers that need it spoken pass a label.
 */
it('is invisible to assistive technology by default', () => {
  render(
    <ThemeProvider>
      <Icon name="bell" />
    </ThemeProvider>,
  );

  expect(screen.queryByLabelText('bell')).toBeNull();
});

it('is announced when given a label', () => {
  render(
    <ThemeProvider>
      <Icon name="bell" accessibilityLabel="Notifications" />
    </ThemeProvider>,
  );

  expect(screen.getByLabelText('Notifications')).toBeTruthy();
});
