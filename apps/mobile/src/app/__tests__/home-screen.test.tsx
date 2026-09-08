import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { ThemeProvider } from '@/theme';

import HomeScreen from '../(tabs)/home';

/**
 * Today: one timeline across every circle, narrowed by a person filter.
 *
 * The assignment sections this replaced ("Yours to do" / "Also today") are
 * gone. Assignment is a tag on a row now: a family member caring alone assigns
 * nothing to themselves, so it was heading a section that was always empty and
 * pushing the whole day under a second one.
 */

const mockToday = jest.fn();
const mockSeniors = jest.fn();
const mockSummaries = jest.fn();
const mockSettle = jest.fn();

jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ isSignedIn: true, isRestoring: false }),
}));
jest.mock('@/features/today/use-today', () => ({
  useToday: () => mockToday(),
  todayKeys: { all: ['today'] },
}));
jest.mock('@/features/today/settle', () => ({
  settleAgendaItem: (...args: unknown[]) => mockSettle(...args),
}));
jest.mock('@/features/seniors/use-seniors', () => ({ useSeniors: () => mockSeniors() }));
jest.mock('@/features/seniors/use-senior-summaries', () => ({
  useSeniorSummaries: () => mockSummaries(),
}));
jest.mock('@/features/sync/use-sync', () => ({ useOfflineSync: jest.fn() }));
jest.mock('expo-router', () => ({
  router: { push: jest.fn() },
  Redirect: () => null,
  Stack: { Screen: () => null },
}));

function renderScreen() {
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <SafeAreaProvider
        initialMetrics={{
          frame: { x: 0, y: 0, width: 390, height: 844 },
          insets: { top: 47, left: 0, right: 0, bottom: 34 },
        }}
      >
        <QueryClientProvider client={new QueryClient()}>
          <ThemeProvider>{children}</ThemeProvider>
        </QueryClientProvider>
      </SafeAreaProvider>
    );
  }

  return render(<HomeScreen />, { wrapper: Wrapper });
}

function item(overrides = {}) {
  return {
    kind: 'dose',
    id: 'dose-1',
    seniorId: 'senior-1',
    seniorName: 'Amina Bibi',
    timezone: 'UTC',
    title: 'Metformin',
    detail: '500 mg',
    scheduledFor: '2026-09-08T09:00:00.000Z',
    status: 'pending',
    assignedToMe: false,
    ...overrides,
  };
}

function person(id: string, name: string, isSelf = false) {
  return { id, displayName: name, isSelf, timezone: 'UTC' };
}

beforeEach(() => {
  mockSettle.mockReset().mockResolvedValue('recorded');
  mockToday.mockReturnValue({ data: [], isPending: false, isError: false });
  mockSeniors.mockReturnValue({ data: [person('senior-1', 'Amina Bibi')] });
  mockSummaries.mockReturnValue({ data: [] });
});

/** "Today" promised a day and showed a menu. It has to say which day it is. */
it("names today's date", () => {
  renderScreen();

  const today = new Date().toLocaleDateString(undefined, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
  });

  expect(screen.getByText(today)).toBeTruthy();
});

/**
 * The bug this screen was rebuilt for: Today read only tasks assigned to the
 * reader, so a family member caring alone saw "Nothing needs you" while their
 * mother had four doses due.
 */
it('shows care that nobody has been assigned', () => {
  mockToday.mockReturnValue({ data: [item()], isPending: false, isError: false });
  renderScreen();

  expect(screen.getByText('Metformin')).toBeTruthy();
});

it('says so when the day really is clear', () => {
  renderScreen();

  expect(screen.getByText('Nothing needs you')).toBeTruthy();
});

/** Sign-out moved to Settings and must not drift back onto Home. */
it('does not offer sign out', () => {
  renderScreen();

  expect(screen.queryByRole('button', { name: 'Sign out' })).toBeNull();
});

it('offers a way back when the day could not be loaded', () => {
  const refetch = jest.fn();
  mockToday.mockReturnValue({ data: undefined, isPending: false, isError: true, refetch });
  renderScreen();

  fireEvent.press(screen.getByRole('button', { name: 'Try again' }));
  expect(refetch).toHaveBeenCalled();
});

// --- the person filter --------------------------------------------------------

function twoCircles() {
  mockSeniors.mockReturnValue({
    data: [person('senior-1', 'Amina Bibi'), person('senior-2', 'Yusuf Khan')],
  });
  mockToday.mockReturnValue({
    data: [
      item(),
      item({
        id: 'dose-2',
        seniorId: 'senior-2',
        seniorName: 'Yusuf Khan',
        title: 'Atorvastatin',
        scheduledFor: '2026-09-08T17:00:00.000Z',
      }),
    ],
    isPending: false,
    isError: false,
  });
}

it('shows the whole circle by default', () => {
  twoCircles();
  renderScreen();

  expect(screen.getByRole('header', { name: 'Today' })).toBeTruthy();
  expect(screen.getByText('Metformin')).toBeTruthy();
  expect(screen.getByText('Atorvastatin')).toBeTruthy();
});

it('narrows the day to the person tapped', () => {
  twoCircles();
  renderScreen();

  fireEvent.press(screen.getByRole('tab', { name: 'Yusuf Khan' }));

  expect(screen.getByText('Atorvastatin')).toBeTruthy();
  expect(screen.queryByText('Metformin')).toBeNull();
});

/**
 * The name moves to the heading, which is what frees the row to say what the
 * dose actually is instead of repeating whose it is.
 */
it('names the person in the heading once narrowed', () => {
  twoCircles();
  renderScreen();

  fireEvent.press(screen.getByRole('tab', { name: 'Yusuf Khan' }));

  expect(screen.getByRole('header', { name: 'Yusuf Khan' })).toBeTruthy();
  // The current stop also carries how soon it is due, so match the dosage
  // within the line rather than as the whole of it.
  expect(screen.getByText(/500 mg/, { includeHiddenElements: true })).toBeTruthy();
});

it('goes back to the whole circle', () => {
  twoCircles();
  renderScreen();

  fireEvent.press(screen.getByRole('tab', { name: 'Yusuf Khan' }));
  fireEvent.press(screen.getByRole('tab', { name: 'Everyone' }));

  expect(screen.getByText('Metformin')).toBeTruthy();
  expect(screen.getByText('Atorvastatin')).toBeTruthy();
});

/**
 * Not "nothing needs you" — the day is not clear, it is only clear for this one
 * person, and saying otherwise would be wrong the moment they tap Everyone.
 */
it('says the chosen person is clear without claiming the whole day is', () => {
  twoCircles();
  mockSeniors.mockReturnValue({
    data: [
      person('senior-1', 'Amina Bibi'),
      person('senior-2', 'Yusuf Khan'),
      person('senior-3', 'Fatima Noor'),
    ],
  });
  renderScreen();

  fireEvent.press(screen.getByRole('tab', { name: 'Fatima Noor' }));

  expect(screen.getByText('Nothing due for Fatima Noor')).toBeTruthy();
  expect(screen.queryByText('Nothing needs you')).toBeNull();
});

// --- what has slipped ----------------------------------------------------------

it('names what slipped at the top of the day', () => {
  mockToday.mockReturnValue({
    data: [item({ status: 'missed', title: 'Amlodipine' })],
    isPending: false,
    isError: false,
  });
  renderScreen();

  expect(screen.getByText('Amlodipine was missed')).toBeTruthy();
});

it('says nothing at the top when the day is on track', () => {
  mockToday.mockReturnValue({ data: [item()], isPending: false, isError: false });
  renderScreen();

  expect(screen.queryByRole('alert')).toBeNull();
});

// --- assignment ----------------------------------------------------------------

/**
 * Assignment orders and marks the day; it never hides any of it. A professional
 * working a round needs their own work called out, and everyone else needs the
 * rest of the day to still be there.
 */
it('marks the work that is yours without hiding anyone else’s', () => {
  mockToday.mockReturnValue({
    data: [item({ assignedToMe: true }), item({ id: 'dose-2', title: 'Aspirin' })],
    isPending: false,
    isError: false,
  });
  renderScreen();

  expect(screen.getByText('Yours')).toBeTruthy();
  expect(screen.getByText('Aspirin')).toBeTruthy();
});

// --- recording an outcome --------------------------------------------------------

it('records a dose from the day itself', () => {
  mockToday.mockReturnValue({ data: [item()], isPending: false, isError: false });
  renderScreen();

  fireEvent.press(screen.getByRole('button', { name: 'Mark as taken' }));

  expect(mockSettle).toHaveBeenCalledWith(
    expect.anything(),
    { kind: 'dose', id: 'dose-1', seniorId: 'senior-1' },
    'done',
  );
});

it('asks a task to be marked done rather than taken', () => {
  mockToday.mockReturnValue({
    data: [item({ kind: 'task', id: 'task-1', title: 'Morning walk' })],
    isPending: false,
    isError: false,
  });
  renderScreen();

  expect(screen.getByRole('button', { name: 'Mark as done' })).toBeTruthy();
});

/** Only the current stop carries buttons; the rest of the day is a list. */
it('offers the action on the current thing only', () => {
  mockToday.mockReturnValue({
    data: [
      item(),
      item({ id: 'dose-2', title: 'Aspirin', scheduledFor: '2026-09-08T17:00:00.000Z' }),
    ],
    isPending: false,
    isError: false,
  });
  renderScreen();

  expect(screen.getAllByRole('button', { name: 'Mark as taken' })).toHaveLength(1);
});
