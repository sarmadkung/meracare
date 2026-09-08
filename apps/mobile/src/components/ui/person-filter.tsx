import type { ReactNode } from 'react';
import { Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { useTheme } from '@/theme';

import { Icon } from './icon';
import { initials } from './avatar';
import { Text } from './text';

/** One face in the strip. */
export interface FilterPerson {
  seniorId: string;
  name: string;
  isSelf: boolean;
  needsAttention: boolean;
}

export interface PersonFilterProps {
  people: FilterPerson[];
  /** null means everyone. */
  selected: string | null;
  onSelect: (seniorId: string | null) => void;
}

/**
 * Narrows the day to one person.
 *
 * A filter rather than a destination: tapping a face changes the list below and
 * never leaves the screen, so a professional working a round can flick between
 * clients without going back each time. Opening a screen instead would put a
 * second, weaker senior dashboard one tap from Today, and that screen already
 * exists on the Circle tab.
 *
 * It hides itself below two people. The family member caring for one parent is
 * the common case, and a control with a single choice is not a control.
 */
export function PersonFilter({ people, selected, onSelect }: PersonFilterProps) {
  const theme = useTheme();

  if (people.length < 2) return null;

  return (
    <ScrollView
      horizontal
      accessibilityRole="tablist"
      showsHorizontalScrollIndicator={false}
      contentContainerStyle={{ gap: theme.spacing.md, paddingVertical: theme.spacing.sm }}
    >
      <Choice
        label="Everyone"
        selected={selected === null}
        onPress={() => onSelect(null)}
        glyph={<Icon name="people" color={theme.colors.textSecondary} />}
      />

      {people.map((person) => (
        <Choice
          key={person.seniorId}
          label={person.name}
          // Colour is never the only carrier of meaning (docs/18): the dot is
          // drawn, and the same fact is spoken.
          spokenLabel={person.needsAttention ? `${person.name}, needs attention` : undefined}
          selected={selected === person.seniorId}
          marked={person.needsAttention}
          onPress={() => onSelect(person.seniorId)}
        />
      ))}
    </ScrollView>
  );
}

function Choice({
  label,
  spokenLabel,
  selected,
  marked,
  glyph,
  onPress,
}: {
  label: string;
  spokenLabel?: string;
  selected: boolean;
  marked?: boolean;
  glyph?: ReactNode;
  onPress: () => void;
}) {
  const theme = useTheme();

  return (
    <Pressable
      // A tab rather than a button: it changes what the list below shows and
      // never leaves the screen. It also keeps a person's chip from colliding
      // with a row that names the same person.
      accessibilityRole="tab"
      accessibilityLabel={spokenLabel ?? label}
      accessibilityState={{ selected }}
      onPress={onPress}
      style={({ pressed }) => [styles.choice, { opacity: pressed ? 0.7 : 1 }]}
    >
      <View
        style={[
          styles.disc,
          {
            backgroundColor: selected ? theme.colors.primary : theme.colors.surfaceMuted,
            borderColor: selected ? theme.colors.primary : theme.colors.border,
            borderRadius: theme.radii.pill,
          },
        ]}
      >
        {glyph ?? (
          <Text
            variant="bodyStrong"
            style={{ color: selected ? theme.colors.onPrimary : theme.colors.textSecondary }}
          >
            {initials(label)}
          </Text>
        )}

        {marked === true ? (
          <View
            style={[
              styles.dot,
              { backgroundColor: theme.colors.danger, borderColor: theme.colors.background },
            ]}
          />
        ) : null}
      </View>

      <Text
        variant="secondary"
        numberOfLines={1}
        style={{
          color: selected ? theme.colors.primary : theme.colors.textSecondary,
          textAlign: 'center',
        }}
      >
        {label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  choice: { alignItems: 'center', gap: 6, width: 68 },
  disc: {
    alignItems: 'center',
    borderWidth: 2,
    height: 48,
    justifyContent: 'center',
    width: 48,
  },
  dot: {
    borderRadius: 7,
    borderWidth: 2,
    height: 14,
    position: 'absolute',
    right: -1,
    top: -1,
    width: 14,
  },
});
