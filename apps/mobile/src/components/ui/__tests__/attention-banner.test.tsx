import { fireEvent, render, screen } from '@testing-library/react-native';

import { AttentionBanner } from '../attention-banner';
import { ThemeProvider } from '@/theme';

/**
 * What has slipped, said at the top of the day in words.
 *
 * A missed dose deserves to be named, not inferred from an amber pill halfway
 * down a list.
 */

function renderBanner(props: Partial<Parameters<typeof AttentionBanner>[0]> = {}) {
  return render(
    <ThemeProvider>
      <AttentionBanner title="Amlodipine was missed" detail="Due 08:00" {...props} />
    </ThemeProvider>,
  );
}

it('says what slipped and when it was due', () => {
  renderBanner();

  expect(screen.getByText('Amlodipine was missed')).toBeTruthy();
  expect(screen.getByText('Due 08:00')).toBeTruthy();
});

/**
 * Assistive technology should reach this before the list, the same way the eye
 * does — but as an announcement, not as something to be tapped past.
 */
it('is announced as an alert', () => {
  renderBanner();

  expect(screen.getByRole('alert')).toBeTruthy();
});

it('is only tappable when there is somewhere to go', () => {
  renderBanner();
  expect(screen.queryByRole('button')).toBeNull();

  const onPress = jest.fn();
  screen.rerender(
    <ThemeProvider>
      <AttentionBanner title="2 need attention" detail="Amlodipine · Bathe" onPress={onPress} />
    </ThemeProvider>,
  );

  fireEvent.press(screen.getByRole('button', { name: '2 need attention, Amlodipine · Bathe' }));
  expect(onPress).toHaveBeenCalled();
});
