import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { Text as RNText } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { ThemeProvider } from '@/theme';

import { Screen } from '../screen';

/** Screen reads safe-area insets, so it needs a provider with known metrics. */
function Wrapper({ children }: { children: ReactNode }) {
  return (
    <SafeAreaProvider
      initialMetrics={{
        frame: { x: 0, y: 0, width: 390, height: 844 },
        insets: { top: 47, left: 0, right: 0, bottom: 34 },
      }}
    >
      <ThemeProvider>{children}</ThemeProvider>
    </SafeAreaProvider>
  );
}

function styleOf(testID: string) {
  return JSON.stringify(screen.getByTestId(testID).props.style);
}

/**
 * A list needs a tighter rhythm than a form. Cards carry their own padding, so
 * the single uniform gap left lists looking loose and unrelated to each other
 * while forms stayed cramped.
 */
it('gives a list a tighter rhythm than a form', () => {
  const { rerender } = render(
    <Wrapper>
      <Screen variant="form" testID="s">
        <RNText>x</RNText>
      </Screen>
    </Wrapper>,
  );
  const form = styleOf('s');

  rerender(
    <Wrapper>
      <Screen variant="list" testID="s">
        <RNText>x</RNText>
      </Screen>
    </Wrapper>,
  );

  expect(styleOf('s')).not.toBe(form);
});

/** Thirty existing screens pass no variant and must not shift under them. */
it('defaults to the rhythm every existing screen was built against', () => {
  const { rerender } = render(
    <Wrapper>
      <Screen testID="s">
        <RNText>x</RNText>
      </Screen>
    </Wrapper>,
  );
  const bare = styleOf('s');

  rerender(
    <Wrapper>
      <Screen variant="form" testID="s">
        <RNText>x</RNText>
      </Screen>
    </Wrapper>,
  );

  expect(styleOf('s')).toBe(bare);
});

/** The scrollable branch dropped every prop it was given, testID included. */
it('forwards props when scrollable', () => {
  render(
    <Wrapper>
      <Screen scrollable testID="scroller">
        <RNText>x</RNText>
      </Screen>
    </Wrapper>,
  );

  expect(screen.getByTestId('scroller')).toBeTruthy();
});
