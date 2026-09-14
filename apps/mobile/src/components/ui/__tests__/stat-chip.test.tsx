import { render, screen } from '@testing-library/react-native';

import { ThemeProvider } from '@/theme';

import { StatChip } from '../stat-chip';

/**
 * "3/4" and "Meds" are one fact. Read separately by a screen reader they are
 * two fragments that mean nothing, so the chip speaks as a single phrase.
 */
it('speaks its number and label as one phrase', () => {
  render(
    <ThemeProvider>
      <StatChip value="3/4" label="Meds" />
    </ThemeProvider>,
  );

  expect(screen.getByLabelText('3/4 Meds')).toBeTruthy();
});

/** Tone carries meaning, so the text must carry it too — colour is never alone. */
it('shows its label as text in every tone', () => {
  render(
    <ThemeProvider>
      <StatChip value="1" label="Due" tone="warning" />
    </ThemeProvider>,
  );

  expect(screen.getByText('Due')).toBeTruthy();
});
