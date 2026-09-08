import { render, screen } from '@testing-library/react-native';

import { ThemeProvider } from '@/theme';

import { Avatar, initials } from '../avatar';

it.each([
  ['James Miller', 'JM'],
  ['James', 'J'],
  ['  mary jane  watson ', 'MW'],
  ['', '?'],
])('reduces %p to %p', (name, expected) => {
  expect(initials(name)).toBe(expected);
});

/**
 * The name is always rendered beside the avatar, so speaking the initials too
 * would announce the same person twice. It is drawn, but not spoken — which is
 * why finding it requires reaching past the accessibility tree.
 */
it('draws initials that assistive technology cannot see', () => {
  render(
    <ThemeProvider>
      <Avatar name="James Miller" />
    </ThemeProvider>,
  );

  expect(screen.getByText('JM', { includeHiddenElements: true })).toBeTruthy();
  expect(screen.queryByText('JM')).toBeNull();
});
