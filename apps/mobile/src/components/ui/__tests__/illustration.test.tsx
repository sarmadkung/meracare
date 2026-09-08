import { render, screen } from '@testing-library/react-native';

import { Illustration, type IllustrationName } from '../illustration';

const names: IllustrationName[] = [
  'welcome',
  'addSenior',
  'allCaughtUp',
  'careCircle',
  'communication',
];

it.each(names)('renders the %s illustration through its semantic name', (name) => {
  render(<Illustration name={name} accessibilityLabel={`${name} artwork`} />);

  expect(screen.getByLabelText(`${name} artwork`)).toBeTruthy();
});

it('is decorative by default when nearby copy already explains it', () => {
  render(<Illustration name="welcome" />);

  expect(screen.queryByRole('image')).toBeNull();
});
