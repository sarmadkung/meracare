import { fireEvent, render, screen } from '@testing-library/react-native';

import { AgendaRow } from '../agenda-row';
import { ThemeProvider } from '@/theme';

/**
 * One stop on the day's timeline: a time in the gutter, a node on the rail, and
 * what is happening.
 */

function renderRow(props: Partial<Parameters<typeof AgendaRow>[0]> = {}) {
  return render(
    <ThemeProvider>
      <AgendaRow
        time="14:00"
        showTime
        icon="pill"
        title="Metformin"
        detail="500 mg"
        tone="upcoming"
        status={null}
        onPress={jest.fn()}
        {...props}
      />
    </ThemeProvider>,
  );
}

it('reads as one thing, not as a time and a row', () => {
  renderRow();

  expect(screen.getByRole('button', { name: '14:00, Metformin, 500 mg' })).toBeTruthy();
});

it('speaks the status when there is one', () => {
  renderRow({ status: 'Missed', tone: 'attention' });

  expect(screen.getByRole('button', { name: '14:00, Metformin, 500 mg, Missed' })).toBeTruthy();
});

/**
 * The gutter prints a shared time once, but a screen reader has no gutter — a
 * row that says only "Metformin" leaves the listener without the time.
 */
it('still speaks the time on a row that does not print one', () => {
  renderRow({ showTime: false });

  expect(screen.getByRole('button', { name: '14:00, Metformin, 500 mg' })).toBeTruthy();
  expect(screen.queryAllByText('14:00')).toHaveLength(0);
});

it('opens what it describes', () => {
  const onPress = jest.fn();
  renderRow({ onPress });

  fireEvent.press(screen.getByRole('button', { name: '14:00, Metformin, 500 mg' }));
  expect(onPress).toHaveBeenCalled();
});

// --- recording an outcome ----------------------------------------------------

it('offers to record an outcome only where one is given', () => {
  renderRow();
  expect(screen.queryByRole('button', { name: 'Mark as taken' })).toBeNull();

  screen.rerender(
    <ThemeProvider>
      <AgendaRow
        time="14:00"
        showTime
        icon="pill"
        title="Metformin"
        detail="500 mg"
        tone="now"
        status={null}
        onPress={jest.fn()}
        onSettle={jest.fn()}
        settleLabel="Mark as taken"
      />
    </ThemeProvider>,
  );

  expect(screen.getByRole('button', { name: 'Mark as taken' })).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Skip' })).toBeTruthy();
});

it('says which outcome was chosen', () => {
  const onSettle = jest.fn();
  renderRow({ tone: 'now', onSettle, settleLabel: 'Mark as taken' });

  fireEvent.press(screen.getByRole('button', { name: 'Mark as taken' }));
  expect(onSettle).toHaveBeenCalledWith('done');

  fireEvent.press(screen.getByRole('button', { name: 'Skip' }));
  expect(onSettle).toHaveBeenCalledWith('skipped');
});

/**
 * A second tap while the first is in flight would record the same dose twice —
 * which the server treats as idempotent, but which shows the reader a button
 * that appears to do nothing.
 */
it('stops taking taps while one is being recorded', () => {
  const onSettle = jest.fn();
  renderRow({ tone: 'now', onSettle, settleLabel: 'Mark as taken', settling: true });

  fireEvent.press(screen.getByRole('button', { name: 'Mark as taken' }));
  expect(onSettle).not.toHaveBeenCalled();
});
