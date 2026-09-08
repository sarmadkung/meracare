import { render, screen } from '@testing-library/react-native';
import { Text as RNText } from 'react-native';

import { ThemeProvider } from '@/theme';

import { SectionHeader } from '../section-header';

it('announces itself as a heading', () => {
  render(
    <ThemeProvider>
      <SectionHeader title="Up next" />
    </ThemeProvider>,
  );

  expect(screen.getByRole('header', { name: 'Up next' })).toBeTruthy();
});

it('renders a trailing action beside the title', () => {
  render(
    <ThemeProvider>
      <SectionHeader title="Up next" action={<RNText>See all</RNText>} />
    </ThemeProvider>,
  );

  expect(screen.getByText('See all')).toBeTruthy();
});
