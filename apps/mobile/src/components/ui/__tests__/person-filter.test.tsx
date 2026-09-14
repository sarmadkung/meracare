import { fireEvent, render, screen } from '@testing-library/react-native';

import { PersonFilter } from '../person-filter';
import { ThemeProvider } from '@/theme';

/**
 * The strip that narrows Today to one person.
 *
 * It is a filter, not a navigation bar: it changes what the list below shows
 * and never leaves the screen, so a reader can flick between people without
 * going back.
 */

const people = [
  { seniorId: 'me', name: 'Sarmad', isSelf: true, needsAttention: false, timezone: 'Asia/Karachi' },
  {
    seniorId: 'a',
    name: 'Amina Bibi',
    isSelf: false,
    needsAttention: true,
    timezone: 'Asia/Karachi',
  },
  {
    seniorId: 'y',
    name: 'Yusuf Khan',
    isSelf: false,
    needsAttention: false,
    timezone: 'Asia/Karachi',
  },
];

function renderFilter(props: Partial<Parameters<typeof PersonFilter>[0]> = {}) {
  return render(
    <ThemeProvider>
      <PersonFilter people={people} selected={null} onSelect={jest.fn()} {...props} />
    </ThemeProvider>,
  );
}

it('offers everyone and each person by name', () => {
  renderFilter();

  expect(screen.getByRole('tab', { name: 'Everyone' })).toBeTruthy();
  expect(screen.getByRole('tab', { name: 'Amina Bibi' })).toBeTruthy();
});

/**
 * Your own profile is the one face you never have to read a name to recognise,
 * and "You" says what the filter does — the initials still identify the disc.
 */
it('calls your own care yours', () => {
  renderFilter();

  expect(screen.getByRole('tab', { name: 'You' })).toBeTruthy();
  expect(screen.getByText('S')).toBeTruthy();
});

/** Selection is state, not decoration, so it has to be announced as state. */
it('announces which one is chosen', () => {
  renderFilter({ selected: 'a' });

  expect(screen.getByRole('tab', { name: 'Amina Bibi' }).props.accessibilityState).toMatchObject({
    selected: true,
  });
  expect(screen.getByRole('tab', { name: 'Everyone' }).props.accessibilityState).toMatchObject({
    selected: false,
  });
});

it('starts on everyone when nothing is chosen', () => {
  renderFilter({ selected: null });

  expect(screen.getByRole('tab', { name: 'Everyone' }).props.accessibilityState).toMatchObject({
    selected: true,
  });
});

it('reports the person tapped, and null for everyone', () => {
  const onSelect = jest.fn();
  renderFilter({ selected: 'a', onSelect });

  fireEvent.press(screen.getByRole('tab', { name: 'Yusuf Khan' }));
  expect(onSelect).toHaveBeenCalledWith('y');

  fireEvent.press(screen.getByRole('tab', { name: 'Everyone' }));
  expect(onSelect).toHaveBeenCalledWith(null);
});

/**
 * A red dot is colour, and colour is never the only carrier of meaning
 * (docs/18). Whoever is slipping has to be slipping out loud.
 */
it('says in words who needs attention, rather than only marking them', () => {
  renderFilter();

  expect(screen.getByRole('tab', { name: 'Amina Bibi, needs attention' })).toBeTruthy();
});

/**
 * The single-senior family carer is the common case, and a control with one
 * choice is not a control — it is a band of furniture above their day.
 */
it('does not appear when there is nobody to choose between', () => {
  render(
    <ThemeProvider>
      <PersonFilter people={[people[1]!]} selected={null} onSelect={jest.fn()} />
    </ThemeProvider>,
  );

  expect(screen.queryByRole('tab', { name: 'Everyone' })).toBeNull();
});
