import { Pressable, StyleSheet, View } from 'react-native';

import { useTheme } from '@/theme';

import { Button } from './button';
import { Icon, type IconName } from './icon';
import { Text } from './text';

/** Where this stop sits relative to the day. */
export type AgendaTone = 'done' | 'attention' | 'now' | 'upcoming';

/** What the person said happened. */
export type AgendaOutcome = 'done' | 'skipped';

export interface AgendaRowProps {
  /** In the senior's own zone. */
  time: string;
  /** False when the row above is the same moment — the gutter prints it once. */
  showTime: boolean;
  icon: IconName;
  title: string;
  detail: string;
  tone: AgendaTone;
  /** The word on the pill, or null when the state needs no saying. */
  status: string | null;
  /** True for the last stop, which ends the rail rather than continuing it. */
  isLast?: boolean;
  onPress: () => void;
  /** Omit where there is nothing to record — an appointment, or settled work. */
  onSettle?: (outcome: AgendaOutcome) => void;
  /** "Mark as taken" for a dose, "Mark as done" for a task. */
  settleLabel?: string;
  settling?: boolean;
}

/**
 * One stop on the day's timeline.
 *
 * The time sits in a gutter and the rail runs beside it, so the shape of the
 * day is legible before any row is read: what is behind you, where you are, and
 * what is still ahead. Finished work stays on the line rather than disappearing
 * — seeing that the morning went well is most of what a family member opens the
 * app for.
 *
 * Only the current stop carries buttons. Recording an outcome on every row at
 * once would make the screen a form; here there is one obvious thing to do.
 */
export function AgendaRow({
  time,
  showTime,
  icon,
  title,
  detail,
  tone,
  status,
  isLast = false,
  onPress,
  onSettle,
  settleLabel,
  settling = false,
}: AgendaRowProps) {
  const theme = useTheme();

  const nodeColour = {
    done: theme.colors.success,
    attention: theme.colors.warning,
    now: theme.colors.primary,
    upcoming: theme.colors.border,
  }[tone];

  const acting = onSettle !== undefined && settleLabel !== undefined;

  return (
    <View style={styles.stop}>
      {/*
        The gutter prints a shared time once, but a screen reader has no gutter,
        so the spoken name below always carries the time.
      */}
      <View style={styles.gutter}>
        {showTime ? (
          <Text
            variant="secondary"
            style={{
              color: tone === 'now' ? theme.colors.primary : theme.colors.textSecondary,
              textAlign: 'right',
            }}
          >
            {time}
          </Text>
        ) : null}
      </View>

      <View
        accessibilityElementsHidden
        importantForAccessibility="no-hide-descendants"
        style={styles.rail}
      >
        <View
          style={[
            styles.node,
            {
              backgroundColor: tone === 'upcoming' ? theme.colors.background : nodeColour,
              borderColor: nodeColour,
            },
          ]}
        />
        {isLast ? null : <View style={[styles.line, { backgroundColor: theme.colors.border }]} />}
      </View>

      <View style={styles.content}>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={[time, title, detail, status].filter(Boolean).join(', ')}
          onPress={onPress}
          style={({ pressed }) => [
            styles.body,
            {
              // The current stop is a card so it reads as the one thing to do;
              // everything else is quiet.
              backgroundColor: tone === 'now' ? theme.colors.surface : 'transparent',
              borderColor: tone === 'now' ? theme.colors.border : 'transparent',
              borderRadius: theme.radii.lg,
              gap: theme.spacing.md,
              minHeight: theme.minTouchTarget,
              opacity: pressed ? 0.7 : 1,
              padding: theme.spacing.md,
            },
            tone === 'now' ? theme.elevation.card : theme.elevation.none,
          ]}
        >
          <View
            style={[
              styles.glyph,
              {
                backgroundColor: groundFor(tone, theme),
                borderRadius: theme.radii.md,
              },
            ]}
          >
            <Icon name={icon} size={18} color={inkFor(tone, theme)} />
          </View>

          <View style={{ flex: 1, gap: 2 }}>
            <Text
              variant="bodyStrong"
              color={tone === 'done' ? 'muted' : 'primary'}
              style={tone === 'done' ? styles.settled : undefined}
            >
              {title}
            </Text>
            {detail === '' ? null : (
              <Text variant="secondary" color="secondary">
                {detail}
              </Text>
            )}
          </View>

          {status === null ? null : (
            <View
              style={[
                styles.pill,
                { backgroundColor: groundFor(tone, theme), borderRadius: theme.radii.pill },
              ]}
            >
              <Text variant="label" style={{ color: inkFor(tone, theme) }}>
                {status}
              </Text>
            </View>
          )}
        </Pressable>

        {acting ? (
          <View
            style={{ flexDirection: 'row', gap: theme.spacing.sm, marginTop: theme.spacing.sm }}
          >
            <View style={{ flex: 2 }}>
              <Button
                label={settleLabel}
                loading={settling}
                onPress={() => {
                  if (!settling) onSettle('done');
                }}
              />
            </View>
            <View style={{ flex: 1 }}>
              <Button
                variant="secondary"
                label="Skip"
                onPress={() => {
                  if (!settling) onSettle('skipped');
                }}
              />
            </View>
          </View>
        ) : null}
      </View>
    </View>
  );
}

type Theme = ReturnType<typeof useTheme>;

function groundFor(tone: AgendaTone, theme: Theme): string {
  return {
    done: theme.colors.successBackground,
    attention: theme.colors.warningBackground,
    now: theme.colors.primarySubtle,
    upcoming: theme.colors.primarySubtle,
  }[tone];
}

function inkFor(tone: AgendaTone, theme: Theme): string {
  return {
    done: theme.colors.success,
    attention: theme.colors.warning,
    now: theme.colors.primary,
    upcoming: theme.colors.primary,
  }[tone];
}

const styles = StyleSheet.create({
  body: { alignItems: 'center', borderWidth: StyleSheet.hairlineWidth, flexDirection: 'row' },
  content: { flex: 1, paddingBottom: 8 },
  glyph: { alignItems: 'center', height: 34, justifyContent: 'center', width: 34 },
  gutter: { paddingTop: 14, width: 52 },
  line: { flex: 1, width: StyleSheet.hairlineWidth * 2 },
  node: { borderRadius: 6, borderWidth: 2, height: 11, marginTop: 18, width: 11 },
  pill: { paddingHorizontal: 8, paddingVertical: 3 },
  rail: { alignItems: 'center', width: 12 },
  settled: { textDecorationLine: 'line-through' },
  stop: { flexDirection: 'row', gap: 10 },
});
