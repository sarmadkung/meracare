import { fireEvent, render, screen } from '@testing-library/react-native';
import { router } from 'expo-router';

import { ThemeProvider, lightColors } from '@/theme';

import { ListRow } from '../list-row';

jest.mock('expo-router', () => ({ router: { push: jest.fn() } }));

function renderRow() {
  return render(
    <ThemeProvider>
      <ListRow title="James Miller" subtitle="Family care" href="/seniors/1" />
    </ThemeProvider>,
  );
}

const name = 'James Miller, Family care';

it('reads as one button naming the person and their role', () => {
  renderRow();

  expect(screen.getByRole('button', { name })).toBeTruthy();
});

it('navigates when tapped', () => {
  renderRow();

  fireEvent.press(screen.getByRole('button', { name }));

  expect(router.push).toHaveBeenCalledWith('/seniors/1');
});

/**
 * The regression that started the redesign: this row rendered with no surface,
 * no padding and no row direction, so the chevron dropped onto its own line
 * below the name. The cause was `Link asChild` cloning the child and passing
 * its own `style`, which discarded the child's entirely. The row now owns its
 * navigation, so nothing outside it can clobber how it draws.
 */
it('draws itself as a surface, laid out as a row', () => {
  renderRow();

  expect(screen.getByRole('button', { name })).toHaveStyle({
    backgroundColor: lightColors.surface,
    flexDirection: 'row',
  });
});

it('meets the minimum touch target', () => {
  renderRow();

  expect(screen.getByRole('button', { name })).toHaveStyle({ minHeight: 48 });
});
