import { View } from 'react-native';

import { useTheme } from '@/theme';

import { Button } from './button';
import { Illustration, type IllustrationName } from './illustration';
import { Text } from './text';

export interface EmptyStateProps {
  illustration: IllustrationName;
  title: string;
  body: string;
  actionLabel?: string;
  onAction?: () => void;
}

/**
 * What a list says when it has nothing in it.
 *
 * An empty screen that only reports its emptiness is a dead end. Where there is
 * a way forward it is offered right here, because this is where the person is
 * already looking — not behind a button somewhere further down the screen.
 */
export function EmptyState({ illustration, title, body, actionLabel, onAction }: EmptyStateProps) {
  const theme = useTheme();

  return (
    <View
      style={{ alignItems: 'center', gap: theme.spacing.md, paddingVertical: theme.spacing.xl }}
    >
      <Illustration name={illustration} height={150} />

      <Text accessibilityRole="header" variant="sectionHeading">
        {title}
      </Text>

      <Text variant="body" color="secondary" style={{ textAlign: 'center' }}>
        {body}
      </Text>

      {actionLabel !== undefined && onAction !== undefined ? (
        <View style={{ alignSelf: 'stretch', marginTop: theme.spacing.sm }}>
          <Button label={actionLabel} onPress={onAction} />
        </View>
      ) : null}
    </View>
  );
}
