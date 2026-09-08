import { fireEvent, render, screen } from '@testing-library/react-native';

import { ThemeProvider } from '@/theme';

import { EmptyState } from '../empty-state';

it('explains the emptiness and offers the way out', () => {
  const onAction = jest.fn();

  render(
    <ThemeProvider>
      <EmptyState
        illustration="addSenior"
        title="Let's get set up"
        body="Add the person you are caring for — or yourself."
        actionLabel="Get started"
        onAction={onAction}
      />
    </ThemeProvider>,
  );

  expect(screen.getByText("Let's get set up")).toBeTruthy();

  fireEvent.press(screen.getByRole('button', { name: 'Get started' }));

  expect(onAction).toHaveBeenCalledTimes(1);
});

/** Not every empty list has a way forward — an empty inbox is just empty. */
it('renders without an action', () => {
  render(
    <ThemeProvider>
      <EmptyState illustration="allCaughtUp" title="All caught up" body="Nothing needs you." />
    </ThemeProvider>,
  );

  expect(screen.getByText('All caught up')).toBeTruthy();
  expect(screen.queryByRole('button')).toBeNull();
});
