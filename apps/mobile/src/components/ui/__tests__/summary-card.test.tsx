import { fireEvent, render, screen } from '@testing-library/react-native';
import { router } from 'expo-router';

import { ThemeProvider } from '@/theme';

import { SummaryCard } from '../summary-card';

jest.mock('expo-router', () => ({ router: { push: jest.fn() } }));

const stats = [
  { value: '3/4', label: 'Meds', tone: 'brand' as const },
  { value: '2', label: 'Tasks' },
  { value: '1', label: 'Due', tone: 'warning' as const },
];

const name = 'James Miller, Family care';

function renderCard() {
  return render(
    <ThemeProvider>
      <SummaryCard name="James Miller" role="Family care" href="/seniors/1" stats={stats} />
    </ThemeProvider>,
  );
}

it('names the person and their role', () => {
  renderCard();

  expect(screen.getByRole('button', { name })).toBeTruthy();
});

it('opens that person when tapped', () => {
  renderCard();

  fireEvent.press(screen.getByRole('button', { name }));

  expect(router.push).toHaveBeenCalledWith('/seniors/1');
});

/**
 * A list of names cannot answer the question a caregiver opens the app with —
 * does anyone need me right now. The card answers it without a tap.
 */
it('shows every stat it was given', () => {
  renderCard();

  expect(screen.getByLabelText('3/4 Meds')).toBeTruthy();
  expect(screen.getByLabelText('2 Tasks')).toBeTruthy();
  expect(screen.getByLabelText('1 Due')).toBeTruthy();
});

/** Somebody with no permissions granted still gets a card, just without a strip. */
it('renders without stats', () => {
  render(
    <ThemeProvider>
      <SummaryCard name="Ada Lovelace" role="Your own care" href="/seniors/2" stats={[]} />
    </ThemeProvider>,
  );

  expect(screen.getByRole('button', { name: 'Ada Lovelace, Your own care' })).toBeTruthy();
});
