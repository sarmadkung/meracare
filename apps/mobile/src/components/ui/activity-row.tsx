import type { CareEvent } from '@genxcare/contracts';
import {
  careEventCategory,
  careEventDescription,
  careEventTimeLabel,
  careEventTone,
} from '@genxcare/contracts';
import { View } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './text';

export interface ActivityRowProps {
  event: CareEvent;
  /** The senior's timezone, so the time reads as their day. */
  timezone: string;
  /** What to call the person who did it. */
  actorName: string;
}

/**
 * One entry in the activity timeline.
 *
 * The time sits on the left and the sentence on the right, so a column of times
 * reads down the page and somebody scanning for "when" does not have to read
 * every line (plans/phase7.md §13).
 *
 * Deliberately quiet. Most care activity is ordinary — somebody did what they
 * said they would — and a feed where every row is coloured or badged is a feed
 * where nothing stands out. Tone is carried by the category caption and a
 * muted colour, never by colour alone (plans/phase7.md §31).
 */
export function ActivityRow({ event, timezone, actorName }: ActivityRowProps) {
  const theme = useTheme();

  const tone = careEventTone(event.type);
  const toneColor = {
    neutral: theme.colors.textSecondary,
    positive: theme.colors.success,
    muted: theme.colors.textMuted,
  }[tone];

  const description = careEventDescription(event, actorName);
  const time = careEventTimeLabel(event, timezone);

  return (
    <View
      accessible
      accessibilityLabel={`${time}. ${description}.`}
      style={{ flexDirection: 'row', gap: theme.spacing.md }}
    >
      <Text variant="bodyStrong" style={{ minWidth: 56 }}>
        {time}
      </Text>

      <View style={{ flex: 1, gap: theme.spacing.xs }}>
        <Text variant="body">{description}</Text>
        <Text variant="secondary" style={{ color: toneColor }}>
          {careEventCategory(event.type)}
        </Text>
      </View>
    </View>
  );
}
