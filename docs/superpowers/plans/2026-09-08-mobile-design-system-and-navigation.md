# Mobile Design System and Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give GenxCare a four-tab navigation shell, a Neutral Charcoal dark mode, and a shared component layer so the app looks designed and composes consistently.

**Architecture:** Three phases, each independently shippable. Phase 1 builds the token additions and seven UI primitives with no screen changes — consistency comes from shared components, not from repeating work on 30 screens. Phase 2 introduces the `(tabs)` route group, themed header defaults, and the Settings stack that finally houses sign-out. Phase 3 rebuilds Today and Circle on the new primitives. Remaining screens (phase 4 in the spec) get their own plan once the component layer is proven in use.

**Tech Stack:** Expo SDK 57.0.13, React Native 0.86.2, expo-router 57 (`expo-router/js-tabs`), TypeScript, `@testing-library/react-native`, Jest with `jest-expo`.

**Spec:** `docs/superpowers/specs/2026-09-08-mobile-design-system-and-navigation-design.md`

## Global Constraints

- Work in `apps/mobile`. All paths below are relative to it unless stated.
- Body text 17pt minimum; primary actions 18pt minimum; touch targets 48dp minimum (`theme.minTouchTarget`).
- Colour is never the only carrier of meaning — status always has a text label.
- No hardcoded hex outside `src/theme/`. The two exceptions stay: `apple-button.tsx` and `google-button.tsx` carry Apple and Google brand values that their guidelines require.
- No hardcoded `fontSize` anywhere outside `src/theme/tokens.ts`.
- Screens never import `@expo/vector-icons` directly — only `components/ui/icon.tsx` may.
- Light-mode brand teal is `#0F766E`; dark-mode brand teal is `#14B8A6`. Same role, different value, deliberately.
- Every existing route keeps its current URL. `(tabs)` is a group and does not appear in paths.
- TDD: write the test, run it, watch it fail for the right reason, then implement.
- Gates before every commit: `pnpm test`, `pnpm typecheck`, `pnpm lint`, `pnpm exec prettier --check "src/**/*.{ts,tsx}"`.
- Components are tested at the boundary a user meets — role, accessible name, visible text. Never internal structure.

---

## File Structure

**Phase 1 — created:**

| File | Responsibility |
|---|---|
| `src/components/ui/icon.tsx` | semantic name → glyph; the only importer of `@expo/vector-icons` |
| `src/components/ui/list-row.tsx` | tappable row; owns its own navigation |
| `src/components/ui/avatar.tsx` | initials on a themed disc |
| `src/components/ui/stat-chip.tsx` | one number + label on a tinted ground |
| `src/components/ui/section-header.tsx` | the `UP NEXT` eyebrow |
| `src/components/ui/summary-card.tsx` | person card: header row + stat strip |
| `src/components/ui/empty-state.tsx` | illustration + heading + body + action |

**Phase 1 — modified:** `src/theme/tokens.ts`, `src/theme/theme-provider.tsx`, `src/theme/index.ts`, `src/components/ui/screen.tsx`, `src/components/ui/index.ts`, `package.json`.

**Phase 2 — created:** `src/app/(tabs)/_layout.tsx`, `src/app/(tabs)/circle.tsx`, `src/app/(tabs)/settings/_layout.tsx`, `src/app/(tabs)/settings/index.tsx`, `src/app/(tabs)/settings/profile.tsx`.

**Phase 2 — moved:** `src/app/home.tsx` → `src/app/(tabs)/home.tsx`; `src/app/notifications.tsx` → `src/app/(tabs)/notifications.tsx`; `src/app/settings/notifications.tsx` → `src/app/(tabs)/settings/notifications.tsx`.

---

# Phase 1 — Foundation

### Task 1: Token additions and the Neutral Charcoal dark ramp

**Files:**
- Modify: `src/theme/tokens.ts`
- Modify: `src/theme/theme-provider.tsx`
- Modify: `src/theme/index.ts`
- Test: `src/theme/__tests__/tokens.test.ts` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: `theme.elevation.card`, `theme.elevation.raised`, `theme.elevation.none`; `theme.colors.primarySubtle`; `theme.typography.label`. Every later task uses these.

- [ ] **Step 1: Write the failing test**

Create `src/theme/__tests__/tokens.test.ts`:

```ts
import { darkColors, elevation, lightColors, typography } from '../tokens';

/**
 * Dark mode is Neutral Charcoal: a hueless ground. The previous ramp was
 * teal-tinted, which made dark mode read as a different product from light.
 */
it('uses a hueless ground in dark mode', () => {
  // A neutral grey has equal-ish R, G and B. The old #0B1A19 did not.
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(darkColors.background.slice(i, i + 2), 16));
  expect(Math.max(r, g, b) - Math.min(r, g, b)).toBeLessThanOrEqual(4);
});

/** Brand teal differs by theme on purpose: the light value fails contrast on a dark ground. */
it('keeps a brand teal in both themes, at different values', () => {
  expect(lightColors.primary).toBe('#0F766E');
  expect(darkColors.primary).toBe('#14B8A6');
});

/** A large tinted fill needs a quieter tint than primaryLight, which reads as a mint slab. */
it('defines a subtle brand fill for both themes', () => {
  expect(lightColors.primarySubtle).toBeDefined();
  expect(darkColors.primarySubtle).toBeDefined();
  expect(lightColors.primarySubtle).not.toBe(lightColors.primaryLight);
});

/** The eyebrow label above a section. Below `secondary` in the scale. */
it('defines a label type variant smaller than secondary', () => {
  expect(typography.label.fontSize).toBeLessThan(typography.secondary.fontSize);
});

/** Shadows were written inline or not at all. Named presets make elevation consistent. */
it('defines named elevation presets', () => {
  expect(elevation.none).toEqual({});
  expect(elevation.card.elevation).toBeGreaterThan(0);
  expect(elevation.raised.elevation).toBeGreaterThan(elevation.card.elevation);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec jest src/theme/__tests__/tokens.test.ts`
Expected: FAIL — `elevation` is not exported, `primarySubtle` and `typography.label` are undefined, and `darkColors.background` is `#0B1A19` (channel spread 15, not ≤ 4).

- [ ] **Step 3: Write minimal implementation**

In `src/theme/tokens.ts`, add to `palette`:

```ts
  zinc950: '#131416',
  zinc900: '#1D1E21',
  zinc800: '#27282C',
  zinc700: '#2E2F33',
  zinc400: '#A1A1AA',
  zinc500: '#71717A',
  zinc50: '#F4F4F5',

  tealSubtleLight: '#F0FDF9',
  tealSubtleDark: '#14312E',
```

Add `primarySubtle: string;` to the `ThemeColors` interface, directly under `primaryLight`.

In `lightColors` add `primarySubtle: palette.tealSubtleLight,`.

Replace the ground, surface, text and border entries of `darkColors`:

```ts
  primarySubtle: palette.tealSubtleDark,

  background: palette.zinc950,
  surface: palette.zinc900,
  surfaceMuted: palette.zinc800,

  textPrimary: palette.zinc50,
  textSecondary: palette.zinc400,
  textMuted: palette.zinc500,
  border: palette.zinc700,
```

Delete `slate950`, `slate900`, `slate800` and `slate700` from `palette` — nothing references them once `darkColors` is updated, and leaving them invites drift back to the teal-tinted ground.

Add to `typography`, after `secondary`:

```ts
  label: { fontFamily: 'Inter_600SemiBold', fontSize: 13, lineHeight: 16, letterSpacing: 0.6 },
```

Append to the end of the file:

```ts
/**
 * Named shadow presets.
 *
 * iOS reads the `shadow*` properties and Android reads `elevation`, so both are
 * set on every preset. Screens must never write a shadow inline: an app whose
 * cards lift by different amounts looks unfinished even when nothing else is
 * wrong.
 */
export const elevation = {
  none: {},
  card: {
    shadowColor: '#0B1A19',
    shadowOffset: { width: 0, height: 1 },
    shadowOpacity: 0.07,
    shadowRadius: 3,
    elevation: 1,
  },
  raised: {
    shadowColor: '#0B1A19',
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.1,
    shadowRadius: 12,
    elevation: 4,
  },
} as const;
```

> The two hex values inside `elevation` are shadow colours, not palette roles, and live in `src/theme/` — they satisfy the no-hardcoded-hex rule, which is about screens and components.

In `src/theme/theme-provider.tsx`, import `elevation`, add `elevation: typeof elevation;` to the `Theme` interface, and add `elevation,` to the object returned by `buildTheme`.

In `src/theme/index.ts`, add `elevation` to the export list from `./tokens`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm exec jest src/theme && pnpm typecheck`
Expected: PASS, exit 0.

- [ ] **Step 5: Run the whole suite — the dark ramp is a breaking change**

Run: `pnpm test`
Expected: 343 passing. Any failure names a test asserting on an old dark hex; update it to read from `darkColors` rather than a literal.

- [ ] **Step 6: Commit**

```bash
git add src/theme
git commit -m "Give dark mode a neutral ground and name our shadows"
```

---

### Task 2: The Icon component

**Files:**
- Modify: `package.json`
- Create: `src/components/ui/icon.tsx`
- Modify: `src/components/ui/index.ts`
- Test: `src/components/ui/__tests__/icon.test.tsx` (create)

**Interfaces:**
- Consumes: `useTheme()`.
- Produces: `<Icon name={IconName} size?: number color?: string />` where
  `type IconName = 'chevron' | 'bell' | 'pill' | 'calendar' | 'people' | 'settings' | 'today' | 'task' | 'plus'`.

- [ ] **Step 1: Install the icon library**

Run: `pnpm --filter @genxcare/mobile add @expo/vector-icons`

It ships bundled fonts and needs no native rebuild.

- [ ] **Step 2: Write the failing test**

Create `src/components/ui/__tests__/icon.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react-native';

import { Icon } from '../icon';
import { ThemeProvider } from '@/theme';

/**
 * An icon beside its own label is decoration and must stay silent, or a screen
 * reader announces everything twice. Callers that need it spoken pass a label.
 */
it('is invisible to assistive technology by default', () => {
  render(
    <ThemeProvider>
      <Icon name="bell" />
    </ThemeProvider>,
  );

  expect(screen.queryByLabelText('bell')).toBeNull();
});

it('is announced when given a label', () => {
  render(
    <ThemeProvider>
      <Icon name="bell" accessibilityLabel="Notifications" />
    </ThemeProvider>,
  );

  expect(screen.getByLabelText('Notifications')).toBeTruthy();
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `pnpm exec jest src/components/ui/__tests__/icon.test.tsx`
Expected: FAIL — `Cannot find module '../icon'`.

- [ ] **Step 4: Write minimal implementation**

Create `src/components/ui/icon.tsx`:

```tsx
import Ionicons from '@expo/vector-icons/Ionicons';
import type { ComponentProps } from 'react';

import { useTheme } from '@/theme';

/**
 * The app's icon vocabulary.
 *
 * Screens name what they mean, never a glyph. This is the same containment
 * `Illustration` gives unDraw assets: the set can be swapped in one file, and no
 * screen can invent a one-off icon that belongs to nothing.
 */
export type IconName =
  | 'chevron'
  | 'bell'
  | 'pill'
  | 'calendar'
  | 'people'
  | 'settings'
  | 'today'
  | 'task'
  | 'plus';

const glyphs: Record<IconName, ComponentProps<typeof Ionicons>['name']> = {
  chevron: 'chevron-forward',
  bell: 'notifications-outline',
  pill: 'medkit-outline',
  calendar: 'calendar-outline',
  people: 'people-outline',
  settings: 'settings-outline',
  today: 'home-outline',
  task: 'checkmark-circle-outline',
  plus: 'add',
};

export interface IconProps {
  name: IconName;
  size?: number;
  /** Defaults to the secondary text colour. Pass a theme colour, never a literal. */
  color?: string;
  /**
   * Omit when adjacent text already says the same thing — otherwise a screen
   * reader announces it twice.
   */
  accessibilityLabel?: string;
}

export function Icon({ name, size = 20, color, accessibilityLabel }: IconProps) {
  const theme = useTheme();

  return (
    <Ionicons
      name={glyphs[name]}
      size={size}
      color={color ?? theme.colors.textSecondary}
      accessibilityElementsHidden={accessibilityLabel === undefined}
      importantForAccessibility={accessibilityLabel === undefined ? 'no-hide-descendants' : 'yes'}
      accessibilityLabel={accessibilityLabel}
    />
  );
}
```

Add to `src/components/ui/index.ts`:

```ts
export { Icon, type IconName, type IconProps } from './icon';
```

- [ ] **Step 5: Run test to verify it passes**

Run: `pnpm exec jest src/components/ui/__tests__/icon.test.tsx`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add package.json ../../pnpm-lock.yaml src/components/ui/icon.tsx src/components/ui/index.ts src/components/ui/__tests__/icon.test.tsx
git commit -m "Give the app an icon vocabulary"
```

---

### Task 3: ListRow — and the end of the unstyled-row bug

**Files:**
- Create: `src/components/ui/list-row.tsx`
- Modify: `src/components/ui/index.ts`
- Test: `src/components/ui/__tests__/list-row.test.tsx` (create)

**Interfaces:**
- Consumes: `Icon`, `Text`, `useTheme()`.
- Produces: `<ListRow title subtitle? href? onPress? leading? trailing? accessibilityLabel? />` where `href` is an expo-router `Href`.

**Why this exists:** `home.tsx` wraps a styled `Pressable` in `<Link asChild>`. expo-router clones the child and passes its own `style` through, discarding the child's entire style array — no card, no padding, no `flexDirection: 'row'`, which is why the chevron falls onto its own line. `ListRow` navigates internally with `router.push`, so no caller ever combines `Link asChild` with a styled child again.

- [ ] **Step 1: Write the failing test**

Create `src/components/ui/__tests__/list-row.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react-native';
import { router } from 'expo-router';

import { ListRow } from '../list-row';
import { ThemeProvider, lightColors } from '@/theme';

jest.mock('expo-router', () => ({ router: { push: jest.fn() } }));

function renderRow(props: Partial<Parameters<typeof ListRow>[0]> = {}) {
  return render(
    <ThemeProvider>
      <ListRow title="James Miller" subtitle="Family care" href="/seniors/1" {...props} />
    </ThemeProvider>,
  );
}

it('reads as one button naming the person and their role', () => {
  renderRow();

  expect(screen.getByRole('button', { name: 'James Miller, Family care' })).toBeTruthy();
});

it('navigates when tapped', () => {
  renderRow();

  fireEvent.press(screen.getByRole('button', { name: 'James Miller, Family care' }));

  expect(router.push).toHaveBeenCalledWith('/seniors/1');
});

/**
 * The regression that started the redesign: this row rendered with no surface,
 * no padding and no row direction, so the chevron dropped onto its own line.
 * The cause was `Link asChild` overwriting the child's style, so the row now
 * owns its navigation and its style cannot be clobbered from outside.
 */
it('draws itself as a surface, laid out as a row', () => {
  renderRow();

  expect(screen.getByRole('button', { name: 'James Miller, Family care' })).toHaveStyle({
    backgroundColor: lightColors.surface,
    flexDirection: 'row',
  });
});

it('meets the minimum touch target', () => {
  renderRow();

  expect(screen.getByRole('button', { name: 'James Miller, Family care' })).toHaveStyle({
    minHeight: 48,
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec jest src/components/ui/__tests__/list-row.test.tsx`
Expected: FAIL — `Cannot find module '../list-row'`.

- [ ] **Step 3: Write minimal implementation**

Create `src/components/ui/list-row.tsx`:

```tsx
import { router, type Href } from 'expo-router';
import type { ReactNode } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { useTheme } from '@/theme';

import { Icon } from './icon';
import { Text } from './text';

export interface ListRowProps {
  title: string;
  subtitle?: string;
  /** Where tapping goes. Navigation is handled here, never by wrapping this row. */
  href?: Href;
  onPress?: () => void;
  /** Avatar, icon tile, or anything else that identifies the row. */
  leading?: ReactNode;
  /** Replaces the default chevron — a badge or an action, say. */
  trailing?: ReactNode;
  /** Overrides the spoken name, which is otherwise "title, subtitle". */
  accessibilityLabel?: string;
}

/**
 * One tappable row in a list.
 *
 * It navigates itself. Wrapping a styled row in `<Link asChild>` looks
 * equivalent but is not: expo-router clones the child and passes its own
 * `style`, which silently discards the child's, and the row renders as bare
 * text. That is exactly the bug this component was built to end.
 */
export function ListRow({
  title,
  subtitle,
  href,
  onPress,
  leading,
  trailing,
  accessibilityLabel,
}: ListRowProps) {
  const theme = useTheme();

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={
        accessibilityLabel ?? (subtitle === undefined ? title : `${title}, ${subtitle}`)
      }
      onPress={() => {
        if (onPress !== undefined) onPress();
        else if (href !== undefined) router.push(href);
      }}
      style={({ pressed }) => [
        styles.base,
        {
          backgroundColor: theme.colors.surface,
          borderColor: theme.colors.border,
          borderRadius: theme.radii.lg,
          gap: theme.spacing.md,
          minHeight: theme.minTouchTarget,
          opacity: pressed ? 0.85 : 1,
          padding: theme.spacing.lg,
        },
        theme.elevation.card,
      ]}
    >
      {leading}

      <View style={{ flex: 1, gap: theme.spacing.xs }}>
        <Text variant="bodyStrong">{title}</Text>
        {subtitle === undefined ? null : (
          <Text variant="secondary" color="secondary">
            {subtitle}
          </Text>
        )}
      </View>

      {trailing ?? <Icon name="chevron" color={theme.colors.textMuted} />}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    alignItems: 'center',
    borderWidth: StyleSheet.hairlineWidth,
    flexDirection: 'row',
  },
});
```

Add to `src/components/ui/index.ts`:

```ts
export { ListRow, type ListRowProps } from './list-row';
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm exec jest src/components/ui/__tests__/list-row.test.tsx`
Expected: PASS, 4 tests.

- [ ] **Step 5: Commit**

```bash
git add src/components/ui/list-row.tsx src/components/ui/index.ts src/components/ui/__tests__/list-row.test.tsx
git commit -m "Add ListRow, which navigates itself"
```

---

### Task 4: Avatar

**Files:**
- Create: `src/components/ui/avatar.tsx`
- Modify: `src/components/ui/index.ts`
- Test: `src/components/ui/__tests__/avatar.test.tsx` (create)

**Interfaces:**
- Consumes: `useTheme()`, `Text`.
- Produces: `<Avatar name: string size?: number />`, plus `export function initials(name: string): string`.

- [ ] **Step 1: Write the failing test**

Create `src/components/ui/__tests__/avatar.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react-native';

import { Avatar, initials } from '../avatar';
import { ThemeProvider } from '@/theme';

it.each([
  ['James Miller', 'JM'],
  ['James', 'J'],
  ['  mary jane  watson ', 'MW'],
  ['', '?'],
])('reduces %p to %p', (name, expected) => {
  expect(initials(name)).toBe(expected);
});

/** The name is always beside the avatar, so speaking it here would double it. */
it('stays silent for assistive technology', () => {
  render(
    <ThemeProvider>
      <Avatar name="James Miller" />
    </ThemeProvider>,
  );

  expect(screen.getByText('JM')).toBeTruthy();
  expect(screen.queryByLabelText('James Miller')).toBeNull();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec jest src/components/ui/__tests__/avatar.test.tsx`
Expected: FAIL — `Cannot find module '../avatar'`.

- [ ] **Step 3: Write minimal implementation**

Create `src/components/ui/avatar.tsx`:

```tsx
import { View } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './text';

/**
 * First and last initials. One word gives one letter; nothing usable gives "?",
 * because a blank disc beside a row reads as a rendering failure.
 */
export function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return '?';

  const first = words[0]?.[0] ?? '';
  const last = words.length > 1 ? (words[words.length - 1]?.[0] ?? '') : '';

  return `${first}${last}`.toUpperCase();
}

export interface AvatarProps {
  name: string;
  size?: number;
}

/** A person, as initials on a brand-tinted disc. Decorative: the name is always adjacent. */
export function Avatar({ name, size = 44 }: AvatarProps) {
  const theme = useTheme();

  return (
    <View
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      style={{
        alignItems: 'center',
        backgroundColor: theme.colors.primaryLight,
        borderRadius: theme.radii.pill,
        height: size,
        justifyContent: 'center',
        width: size,
      }}
    >
      <Text variant="bodyStrong" style={{ color: theme.colors.primaryDark }}>
        {initials(name)}
      </Text>
    </View>
  );
}
```

Add to `src/components/ui/index.ts`:

```ts
export { Avatar, initials, type AvatarProps } from './avatar';
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm exec jest src/components/ui/__tests__/avatar.test.tsx`
Expected: PASS, 5 tests.

- [ ] **Step 5: Commit**

```bash
git add src/components/ui/avatar.tsx src/components/ui/index.ts src/components/ui/__tests__/avatar.test.tsx
git commit -m "Add Avatar"
```

---

### Task 5: StatChip and SectionHeader

**Files:**
- Create: `src/components/ui/stat-chip.tsx`
- Create: `src/components/ui/section-header.tsx`
- Modify: `src/components/ui/index.ts`
- Test: `src/components/ui/__tests__/stat-chip.test.tsx` (create)
- Test: `src/components/ui/__tests__/section-header.test.tsx` (create)

**Interfaces:**
- Consumes: `useTheme()`, `Text`.
- Produces: `<StatChip value: string label: string tone?: 'brand' | 'neutral' | 'warning' />` and `<SectionHeader title: string action?: ReactNode />`.

- [ ] **Step 1: Write the failing tests**

Create `src/components/ui/__tests__/stat-chip.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react-native';

import { StatChip } from '../stat-chip';
import { ThemeProvider } from '@/theme';

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
```

Create `src/components/ui/__tests__/section-header.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react-native';
import { Text as RNText } from 'react-native';

import { SectionHeader } from '../section-header';
import { ThemeProvider } from '@/theme';

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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm exec jest src/components/ui/__tests__/stat-chip.test.tsx src/components/ui/__tests__/section-header.test.tsx`
Expected: FAIL — both modules missing.

- [ ] **Step 3: Write minimal implementations**

Create `src/components/ui/stat-chip.tsx`:

```tsx
import { View } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './text';

export type StatTone = 'brand' | 'neutral' | 'warning';

export interface StatChipProps {
  value: string;
  label: string;
  tone?: StatTone;
}

/**
 * One number and what it counts.
 *
 * Grouped for assistive technology because "3/4" and "Meds" are a single fact;
 * announced apart they are noise.
 */
export function StatChip({ value, label, tone = 'neutral' }: StatChipProps) {
  const theme = useTheme();

  const grounds: Record<StatTone, string> = {
    brand: theme.colors.primarySubtle,
    neutral: theme.colors.surfaceMuted,
    warning: theme.colors.warningBackground,
  };

  const values: Record<StatTone, string> = {
    brand: theme.colors.primary,
    neutral: theme.colors.textPrimary,
    warning: theme.colors.warning,
  };

  return (
    <View
      accessible
      accessibilityLabel={`${value} ${label}`}
      style={{
        backgroundColor: grounds[tone],
        borderRadius: theme.radii.md,
        flex: 1,
        gap: theme.spacing.xs,
        padding: theme.spacing.md,
      }}
    >
      <Text variant="bodyStrong" style={{ color: values[tone] }}>
        {value}
      </Text>
      <Text variant="label" color={tone === 'warning' ? 'warning' : 'secondary'}>
        {label}
      </Text>
    </View>
  );
}
```

Create `src/components/ui/section-header.tsx`:

```tsx
import type { ReactNode } from 'react';
import { View } from 'react-native';

import { useTheme } from '@/theme';

import { Text } from './text';

export interface SectionHeaderProps {
  title: string;
  /** An optional trailing control, such as "See all". */
  action?: ReactNode;
}

/** The eyebrow above a group of rows. A real heading, so it can be navigated to. */
export function SectionHeader({ title, action }: SectionHeaderProps) {
  const theme = useTheme();

  return (
    <View
      style={{
        alignItems: 'center',
        flexDirection: 'row',
        justifyContent: 'space-between',
        marginTop: theme.spacing.sm,
      }}
    >
      <Text accessibilityRole="header" variant="label" color="secondary">
        {title.toUpperCase()}
      </Text>
      {action}
    </View>
  );
}
```

Add to `src/components/ui/index.ts`:

```ts
export { SectionHeader, type SectionHeaderProps } from './section-header';
export { StatChip, type StatChipProps, type StatTone } from './stat-chip';
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm exec jest src/components/ui/__tests__/stat-chip.test.tsx src/components/ui/__tests__/section-header.test.tsx`
Expected: PASS, 4 tests.

> The heading test asserts on the accessible name `Up next`, not the rendered `UP NEXT`. Casing is presentational — `textTransform` is avoided so the spoken name stays sentence case while the visible text is uppercased in JSX. If the test fails on casing, uppercase in the component as written above and keep the assertion on `title`.

- [ ] **Step 5: Commit**

```bash
git add src/components/ui/stat-chip.tsx src/components/ui/section-header.tsx src/components/ui/index.ts src/components/ui/__tests__/stat-chip.test.tsx src/components/ui/__tests__/section-header.test.tsx
git commit -m "Add StatChip and SectionHeader"
```

---

### Task 6: SummaryCard

**Files:**
- Create: `src/components/ui/summary-card.tsx`
- Modify: `src/components/ui/index.ts`
- Test: `src/components/ui/__tests__/summary-card.test.tsx` (create)

**Interfaces:**
- Consumes: `Avatar`, `Icon`, `StatChip`, `Text`, `useTheme()`, `router`.
- Produces: `<SummaryCard name: string role: string href: Href stats: { value: string; label: string; tone?: StatTone }[] />`.

- [ ] **Step 1: Write the failing test**

Create `src/components/ui/__tests__/summary-card.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react-native';
import { router } from 'expo-router';

import { SummaryCard } from '../summary-card';
import { ThemeProvider } from '@/theme';

jest.mock('expo-router', () => ({ router: { push: jest.fn() } }));

const stats = [
  { value: '3/4', label: 'Meds', tone: 'brand' as const },
  { value: '2', label: 'Tasks' },
  { value: '1', label: 'Due', tone: 'warning' as const },
];

function renderCard() {
  return render(
    <ThemeProvider>
      <SummaryCard name="James Miller" role="Family care" href="/seniors/1" stats={stats} />
    </ThemeProvider>,
  );
}

it('names the person and their role', () => {
  renderCard();

  expect(screen.getByRole('button', { name: 'James Miller, Family care' })).toBeTruthy();
});

it('opens that person when tapped', () => {
  renderCard();

  fireEvent.press(screen.getByRole('button', { name: 'James Miller, Family care' }));

  expect(router.push).toHaveBeenCalledWith('/seniors/1');
});

it('shows every stat it was given', () => {
  renderCard();

  expect(screen.getByLabelText('3/4 Meds')).toBeTruthy();
  expect(screen.getByLabelText('2 Tasks')).toBeTruthy();
  expect(screen.getByLabelText('1 Due')).toBeTruthy();
});

/** A person with no permissions granted still gets a card, just without a stat strip. */
it('renders without stats', () => {
  render(
    <ThemeProvider>
      <SummaryCard name="Ada Lovelace" role="Your own care" href="/seniors/2" stats={[]} />
    </ThemeProvider>,
  );

  expect(screen.getByRole('button', { name: 'Ada Lovelace, Your own care' })).toBeTruthy();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec jest src/components/ui/__tests__/summary-card.test.tsx`
Expected: FAIL — `Cannot find module '../summary-card'`.

- [ ] **Step 3: Write minimal implementation**

Create `src/components/ui/summary-card.tsx`:

```tsx
import { router, type Href } from 'expo-router';
import { Pressable, StyleSheet, View } from 'react-native';

import { useTheme } from '@/theme';

import { Avatar } from './avatar';
import { Icon } from './icon';
import { StatChip, type StatTone } from './stat-chip';
import { Text } from './text';

export interface SummaryStat {
  value: string;
  label: string;
  tone?: StatTone;
}

export interface SummaryCardProps {
  name: string;
  role: string;
  href: Href;
  stats: SummaryStat[];
}

/**
 * One person in a care circle, with how their day is going.
 *
 * This replaces the bare name-and-chevron row on Home. A caregiver opening the
 * app wants to know whether anything needs them, and a list of names cannot
 * answer that — so the answer is on the card rather than one tap further in.
 */
export function SummaryCard({ name, role, href, stats }: SummaryCardProps) {
  const theme = useTheme();

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={`${name}, ${role}`}
      onPress={() => router.push(href)}
      style={({ pressed }) => [
        styles.base,
        {
          backgroundColor: theme.colors.surface,
          borderColor: theme.colors.border,
          borderRadius: theme.radii.lg,
          gap: theme.spacing.md,
          opacity: pressed ? 0.85 : 1,
          padding: theme.spacing.lg,
        },
        theme.elevation.card,
      ]}
    >
      <View style={{ alignItems: 'center', flexDirection: 'row', gap: theme.spacing.md }}>
        <Avatar name={name} />
        <View style={{ flex: 1, gap: theme.spacing.xs }}>
          <Text variant="bodyStrong">{name}</Text>
          <Text variant="secondary" color="secondary">
            {role}
          </Text>
        </View>
        <Icon name="chevron" color={theme.colors.textMuted} />
      </View>

      {stats.length === 0 ? null : (
        <View style={{ flexDirection: 'row', gap: theme.spacing.sm }}>
          {stats.map((stat) => (
            <StatChip key={stat.label} value={stat.value} label={stat.label} tone={stat.tone} />
          ))}
        </View>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    borderWidth: StyleSheet.hairlineWidth,
  },
});
```

Add to `src/components/ui/index.ts`:

```ts
export { SummaryCard, type SummaryCardProps, type SummaryStat } from './summary-card';
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm exec jest src/components/ui/__tests__/summary-card.test.tsx`
Expected: PASS, 4 tests.

- [ ] **Step 5: Commit**

```bash
git add src/components/ui/summary-card.tsx src/components/ui/index.ts src/components/ui/__tests__/summary-card.test.tsx
git commit -m "Add SummaryCard, which says how someone's day is going"
```

---

### Task 7: EmptyState, and Screen variants

**Files:**
- Create: `src/components/ui/empty-state.tsx`
- Modify: `src/components/ui/screen.tsx`
- Modify: `src/components/ui/index.ts`
- Test: `src/components/ui/__tests__/empty-state.test.tsx` (create)
- Test: `src/components/ui/__tests__/screen.test.tsx` (create)

**Interfaces:**
- Consumes: `Button`, `Illustration`, `Text`, `useTheme()`.
- Produces: `<EmptyState illustration: IllustrationName title: string body: string actionLabel?: string onAction?: () => void />`; `<Screen variant?: 'list' | 'form' | 'detail' scrollable? />`.

- [ ] **Step 1: Write the failing tests**

Create `src/components/ui/__tests__/empty-state.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react-native';

import { EmptyState } from '../empty-state';
import { ThemeProvider } from '@/theme';

it('explains the emptiness and offers the way out', () => {
  const onAction = jest.fn();

  render(
    <ThemeProvider>
      <EmptyState
        illustration="addSenior"
        title="Let's get set up"
        body="Add the person you are caring for — or yourself."
        actionLabel="Get started"
        onAction={onAction}
      />
    </ThemeProvider>,
  );

  expect(screen.getByText("Let's get set up")).toBeTruthy();
  fireEvent.press(screen.getByRole('button', { name: 'Get started' }));
  expect(onAction).toHaveBeenCalledTimes(1);
});

/** Not every empty list has an action — an empty inbox is just empty. */
it('renders without an action', () => {
  render(
    <ThemeProvider>
      <EmptyState illustration="allCaughtUp" title="All caught up" body="Nothing needs you." />
    </ThemeProvider>,
  );

  expect(screen.queryByRole('button')).toBeNull();
});
```

Create `src/components/ui/__tests__/screen.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react-native';
import { Text as RNText } from 'react-native';

import { Screen } from '../screen';
import { ThemeProvider } from '@/theme';

/**
 * A list needs tighter rhythm than a form: cards carry their own padding, so
 * the old uniform gap left lists looking loose and unrelated to each other.
 */
it('gives a list a tighter gap than a form', () => {
  const { rerender } = render(
    <ThemeProvider>
      <Screen variant="form" testID="s">
        <RNText>x</RNText>
      </Screen>
    </ThemeProvider>,
  );
  const form = screen.getByTestId('s').props.style;

  rerender(
    <ThemeProvider>
      <Screen variant="list" testID="s">
        <RNText>x</RNText>
      </Screen>
    </ThemeProvider>,
  );
  const list = screen.getByTestId('s').props.style;

  expect(JSON.stringify(list)).not.toBe(JSON.stringify(form));
});

/** Existing screens pass no variant and must not shift. */
it('defaults to the form rhythm every existing screen was built against', () => {
  const { rerender } = render(
    <ThemeProvider>
      <Screen testID="s">
        <RNText>x</RNText>
      </Screen>
    </ThemeProvider>,
  );
  const bare = JSON.stringify(screen.getByTestId('s').props.style);

  rerender(
    <ThemeProvider>
      <Screen variant="form" testID="s">
        <RNText>x</RNText>
      </Screen>
    </ThemeProvider>,
  );

  expect(JSON.stringify(screen.getByTestId('s').props.style)).toBe(bare);
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm exec jest src/components/ui/__tests__/empty-state.test.tsx src/components/ui/__tests__/screen.test.tsx`
Expected: FAIL — `empty-state` missing; `screen.test.tsx` fails because `Screen` ignores `variant` and does not forward `testID` in the scrollable branch.

- [ ] **Step 3: Write minimal implementations**

Create `src/components/ui/empty-state.tsx`:

```tsx
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
 * An empty screen that only says "nothing here" is a dead end. Where there is a
 * way forward it is offered here, because this is where the person is looking.
 */
export function EmptyState({
  illustration,
  title,
  body,
  actionLabel,
  onAction,
}: EmptyStateProps) {
  const theme = useTheme();

  return (
    <View style={{ alignItems: 'center', gap: theme.spacing.md, paddingVertical: theme.spacing.xl }}>
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
```

Rewrite `src/components/ui/screen.tsx`, keeping the existing doc comment and adding:

```tsx
export type ScreenVariant = 'list' | 'form' | 'detail';

export interface ScreenProps extends ViewProps {
  /** Wraps the content in a ScrollView. Use for forms and long content. */
  scrollable?: boolean;
  /**
   * Rhythm for this kind of content. Lists sit tighter because their cards
   * carry their own padding; forms need room between fields.
   */
  variant?: ScreenVariant;
}
```

and inside the component:

```tsx
export function Screen({
  scrollable = false,
  variant = 'form',
  style,
  children,
  ...rest
}: ScreenProps) {
  const theme = useTheme();
  const insets = useSafeAreaInsets();

  const gaps: Record<ScreenVariant, number> = {
    list: theme.spacing.md,
    form: theme.spacing.lg,
    detail: theme.spacing.xl,
  };

  const contentStyle = [
    {
      padding: theme.spacing.lg,
      paddingTop: insets.top + theme.spacing.lg,
      paddingBottom: insets.bottom + theme.spacing.lg,
      gap: gaps[variant],
    },
    style,
  ];

  if (scrollable) {
    return (
      <ScrollView
        style={[styles.fill, { backgroundColor: theme.colors.background }]}
        contentContainerStyle={contentStyle}
        keyboardShouldPersistTaps="handled"
        {...rest}
      >
        {children}
      </ScrollView>
    );
  }

  return (
    <View
      style={[styles.fill, { backgroundColor: theme.colors.background }, contentStyle]}
      {...rest}
    >
      {children}
    </View>
  );
}
```

> `{...rest}` moves onto the `ScrollView` so `testID` and accessibility props reach it. Previously the scrollable branch dropped them silently.

Add to `src/components/ui/index.ts`:

```ts
export { EmptyState, type EmptyStateProps } from './empty-state';
```

and widen the `Screen` export:

```ts
export { Screen, type ScreenProps, type ScreenVariant } from './screen';
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm exec jest src/components/ui`
Expected: PASS.

- [ ] **Step 5: Run every gate — phase 1 is complete**

Run: `pnpm test && pnpm typecheck && pnpm lint && pnpm exec prettier --check "src/**/*.{ts,tsx}"`
Expected: all pass. Test count is 343 plus roughly 21 new.

- [ ] **Step 6: Commit**

```bash
git add src/components/ui
git commit -m "Add EmptyState and give Screen a rhythm per content type"
```

---

# Phase 2 — Navigation

### Task 8: Themed header defaults

**Files:**
- Modify: `src/app/_layout.tsx`

**Interfaces:**
- Consumes: `useTheme()`.
- Produces: a `Stack` whose `screenOptions` supply header colours and fonts, so screens declare only `title`.

- [ ] **Step 1: Extract the Stack into a themed component**

In `src/app/_layout.tsx`, replace `<Stack screenOptions={{ headerShown: false }} />` with `<ThemedStack />`, and add:

```tsx
/**
 * Header styling for every screen, declared once.
 *
 * Screens set only their title. Twenty-four of them previously repeated the
 * full options object — `settings/notifications.tsx` three times over, once per
 * render branch — which is how headers drifted apart from each other.
 */
function ThemedStack() {
  const theme = useTheme();

  return (
    <Stack
      screenOptions={{
        headerShown: false,
        headerStyle: { backgroundColor: theme.colors.background },
        headerTintColor: theme.colors.primary,
        headerTitleStyle: {
          color: theme.colors.textPrimary,
          fontFamily: 'Inter_600SemiBold',
          fontSize: 17,
        },
        headerShadowVisible: false,
        headerBackButtonDisplayMode: 'minimal',
        contentStyle: { backgroundColor: theme.colors.background },
      }}
    />
  );
}
```

`headerShown` stays `false` by default because screens opt in individually today and phase 2 must not change 24 screens at once.

- [ ] **Step 2: Verify nothing broke**

Run: `pnpm test && pnpm typecheck`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add src/app/_layout.tsx
git commit -m "Declare header styling once, at the root"
```

---

### Task 9: The tab shell

**Files:**
- Create: `src/app/(tabs)/_layout.tsx`
- Move: `src/app/home.tsx` → `src/app/(tabs)/home.tsx`
- Move: `src/app/notifications.tsx` → `src/app/(tabs)/notifications.tsx`
- Move: `src/app/settings/notifications.tsx` → `src/app/(tabs)/settings/notifications.tsx`
- Create: `src/app/(tabs)/settings/_layout.tsx`
- Test: `src/app/__tests__/tabs-layout.test.tsx` (create)

**Interfaces:**
- Consumes: `Icon`, `useTheme()`, `useUnreadCount`.
- Produces: routes `/home`, `/circle`, `/notifications`, `/settings`, `/settings/profile`, `/settings/notifications` — every pre-existing path unchanged.

- [ ] **Step 1: Move the files with git so history follows**

```bash
mkdir -p "src/app/(tabs)/settings"
git mv src/app/home.tsx "src/app/(tabs)/home.tsx"
git mv src/app/notifications.tsx "src/app/(tabs)/notifications.tsx"
git mv src/app/settings/notifications.tsx "src/app/(tabs)/settings/notifications.tsx"
rmdir src/app/settings
```

- [ ] **Step 2: Write the failing test**

Create `src/app/__tests__/tabs-layout.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';

import TabsLayout from '../(tabs)/_layout';
import { ThemeProvider } from '@/theme';

jest.mock('@/features/notifications/use-notifications', () => ({ useUnreadCount: () => 3 }));

// Render the tab configuration rather than a real navigator: this test is about
// which tabs exist and what they are called, not about react-navigation.
jest.mock('expo-router/js-tabs', () => {
  const { View } = require('react-native');
  const Tabs = ({ children }: { children: ReactNode }) => <View>{children}</View>;
  Tabs.Screen = ({ options }: { options: { title: string; tabBarBadge?: number } }) => {
    const { Text } = require('react-native');
    return (
      <Text>{options.tabBarBadge === undefined ? options.title : `${options.title} ${options.tabBarBadge}`}</Text>
    );
  };
  return { Tabs };
});

it('offers the four top-level destinations', () => {
  render(
    <ThemeProvider>
      <TabsLayout />
    </ThemeProvider>,
  );

  expect(screen.getByText('Today')).toBeTruthy();
  expect(screen.getByText('Circle')).toBeTruthy();
  expect(screen.getByText('Settings')).toBeTruthy();
});

/** The badge that used to live on a button inside Home belongs on the tab now. */
it('carries the unread count on Alerts', () => {
  render(
    <ThemeProvider>
      <TabsLayout />
    </ThemeProvider>,
  );

  expect(screen.getByText('Alerts 3')).toBeTruthy();
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `pnpm exec jest src/app/__tests__/tabs-layout.test.tsx`
Expected: FAIL — `Cannot find module '../(tabs)/_layout'`.

- [ ] **Step 4: Write minimal implementation**

Create `src/app/(tabs)/_layout.tsx`:

```tsx
import { Tabs } from 'expo-router/js-tabs';

import { Icon, type IconName } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useUnreadCount } from '@/features/notifications/use-notifications';
import { useTheme } from '@/theme';

/**
 * The four places you can be.
 *
 * Today is time-oriented and Circle is people-oriented, which is the split
 * docs/13 already draws between screens 9 and 10. Keeping them apart is what
 * lets one screen set serve a family with one parent and a professional with
 * six clients, instead of two apps.
 *
 * Imported from `expo-router/js-tabs`: the same export from `expo-router` is
 * deprecated in v57.
 */
export default function TabsLayout() {
  const theme = useTheme();
  const { isSignedIn } = useSession();
  const unread = useUnreadCount(isSignedIn);

  const icon =
    (name: IconName) =>
    ({ color, size }: { color: string; size: number }) => <Icon name={name} color={color} size={size} />;

  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarActiveTintColor: theme.colors.primary,
        tabBarInactiveTintColor: theme.colors.textMuted,
        tabBarStyle: {
          backgroundColor: theme.colors.surface,
          borderTopColor: theme.colors.border,
        },
        tabBarLabelStyle: { fontFamily: 'Inter_600SemiBold' },
      }}
    >
      <Tabs.Screen name="home" options={{ title: 'Today', tabBarIcon: icon('today') }} />
      <Tabs.Screen name="circle" options={{ title: 'Circle', tabBarIcon: icon('people') }} />
      <Tabs.Screen
        name="notifications"
        options={{
          title: 'Alerts',
          tabBarIcon: icon('bell'),
          tabBarBadge: unread > 0 ? unread : undefined,
        }}
      />
      <Tabs.Screen name="settings" options={{ title: 'Settings', tabBarIcon: icon('settings') }} />
    </Tabs>
  );
}
```

Create `src/app/(tabs)/settings/_layout.tsx`:

```tsx
import { Stack } from 'expo-router';

/** Settings is a stack so Profile and Notifications push over its index. */
export default function SettingsLayout() {
  return <Stack screenOptions={{ headerShown: true }} />;
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `pnpm exec jest src/app/__tests__/tabs-layout.test.tsx`
Expected: PASS, 2 tests.

- [ ] **Step 6: Commit**

```bash
git add "src/app/(tabs)" src/app/__tests__/tabs-layout.test.tsx
git commit -m "Give the app a tab bar"
```

---

### Task 10: Settings, Profile, and sign-out's new home

**Files:**
- Create: `src/app/(tabs)/settings/index.tsx`
- Create: `src/app/(tabs)/settings/profile.tsx`
- Modify: `src/app/(tabs)/home.tsx`
- Move: `src/features/auth/__tests__/home-screen.test.tsx` → `src/features/auth/__tests__/settings-screen.test.tsx`
- Test: `src/features/auth/__tests__/settings-screen.test.tsx`

**Interfaces:**
- Consumes: `ListRow`, `Button`, `Screen`, `Text`, `useAuthActions()`, `useSession()`.
- Produces: routes `/settings` and `/settings/profile`.

**Removing old-design tests:** `home-screen.test.tsx` exists solely to prove Home shows the sign-out refusal. Sign-out is leaving Home, so the file moves with it rather than being deleted — the coverage is behavioural (a refusal must be visible) and still matters. Its mocks change because Settings renders far less than Home did.

- [ ] **Step 1: Write the failing test**

```bash
git mv src/features/auth/__tests__/home-screen.test.tsx src/features/auth/__tests__/settings-screen.test.tsx
```

Replace its contents:

```tsx
import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import SettingsScreen from '@/app/(tabs)/settings/index';
import { ThemeProvider } from '@/theme';

/**
 * Settings owns the only action in the app that can refuse. A refusal it does
 * not show is indistinguishable from a dead button — the person taps, nothing
 * moves, and there is nothing on screen to act on. This coverage moved here
 * from Home along with the button itself.
 */

const mockAuthActions = jest.fn();

jest.mock('@/features/auth/use-auth-actions', () => ({
  useAuthActions: () => mockAuthActions(),
}));

jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ isSignedIn: true, isRestoring: false, user: { email: 'ada@example.com' } }),
}));

jest.mock('expo-router', () => ({
  router: { push: jest.fn() },
  Stack: { Screen: () => null },
}));

function actions(overrides: Record<string, unknown> = {}) {
  return {
    signOut: jest.fn(),
    pending: null,
    isSubmitting: false,
    error: null,
    clearError: jest.fn(),
    ...overrides,
  };
}

function renderScreen() {
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

  return render(<SettingsScreen />, { wrapper: Wrapper });
}

it('offers sign out', () => {
  mockAuthActions.mockReturnValue(actions());
  renderScreen();

  expect(screen.getByRole('button', { name: 'Sign out' })).toBeTruthy();
});

it('shows why sign-out was refused', () => {
  mockAuthActions.mockReturnValue(
    actions({ error: 'Some offline care updates still need attention.' }),
  );
  renderScreen();

  expect(screen.getByText('Some offline care updates still need attention.')).toBeTruthy();
});

it('says nothing when there is nothing wrong', () => {
  mockAuthActions.mockReturnValue(actions());
  renderScreen();

  expect(screen.queryByRole('alert')).toBeNull();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec jest src/features/auth/__tests__/settings-screen.test.tsx`
Expected: FAIL — `Cannot find module '@/app/(tabs)/settings/index'`.

- [ ] **Step 3: Write minimal implementation**

Create `src/app/(tabs)/settings/index.tsx`:

```tsx
import { Stack, router } from 'expo-router';
import { View } from 'react-native';

import { Button, ListRow, Screen, SectionHeader, Text } from '@/components/ui';
import { useAuthActions } from '@/features/auth/use-auth-actions';
import { useTheme } from '@/theme';

/**
 * Settings (docs/13, screens 24 and 26).
 *
 * Specified in the screen map and never built, which is why sign-out spent its
 * life stranded in the middle of Home between two unrelated buttons.
 */
export default function SettingsScreen() {
  const theme = useTheme();
  const { signOut, isSubmitting, error: signOutError } = useAuthActions();

  return (
    <Screen scrollable variant="list">
      <Stack.Screen options={{ title: 'Settings' }} />

      <SectionHeader title="Account" />
      <ListRow title="Profile" subtitle="Your name and details" href="/settings/profile" />
      <ListRow
        title="Notifications"
        subtitle="Reminders and alerts"
        href="/settings/notifications"
      />

      <View style={{ marginTop: theme.spacing.xl, gap: theme.spacing.md }}>
        {/*
          Sign-out can refuse — a queued care update that has not reached the
          server yet, most often. Left unsaid, the button reads as broken.
        */}
        {signOutError !== null ? (
          <Text accessibilityRole="alert" variant="secondary" color="danger">
            {signOutError}
          </Text>
        ) : null}

        <Button variant="ghost" label="Sign out" onPress={signOut} loading={isSubmitting} />
      </View>
    </Screen>
  );
}
```

Create `src/app/(tabs)/settings/profile.tsx`:

```tsx
import { Stack } from 'expo-router';

import { Card, Screen, Text } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';

/**
 * Profile (docs/13, screen 24).
 *
 * Read-only for now: it shows the identity the app is acting as, which is the
 * question people actually open this screen with. Editing arrives with the
 * account settings work and is deliberately not stubbed here.
 */
export default function ProfileScreen() {
  const { user } = useSession();

  return (
    <Screen scrollable variant="detail">
      <Stack.Screen options={{ title: 'Profile' }} />

      <Card>
        <Text variant="label" color="secondary">
          SIGNED IN AS
        </Text>
        <Text variant="bodyStrong">{user?.email ?? 'Unknown'}</Text>
      </Card>
    </Screen>
  );
}
```

> Check `session-provider.tsx` for the exact shape of `user` before writing this — if it exposes no email, show the display name instead and adjust the test's mock to match. Do not invent a field.

In `src/app/(tabs)/home.tsx`, delete the sign-out block: the `signOutError` conditional, the `Sign out` `Button`, and the `Notification settings` `Button`. Change the destructure on line 31 back to `const { } = ...` — remove the `useAuthActions` import entirely if nothing else on Home uses it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm exec jest src/features/auth && pnpm typecheck`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add "src/app/(tabs)" src/features/auth/__tests__
git commit -m "Give sign-out a home in Settings"
```

---

### Task 11: The Circle tab

**Files:**
- Create: `src/app/(tabs)/circle.tsx`
- Test: `src/app/__tests__/circle-screen.test.tsx` (create)

**Interfaces:**
- Consumes: `SummaryCard`, `EmptyState`, `Button`, `Screen`, `SectionHeader`, `useSeniors()`.
- Produces: route `/circle`.

- [ ] **Step 1: Write the failing test**

Create `src/app/__tests__/circle-screen.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import CircleScreen from '../(tabs)/circle';
import { ThemeProvider } from '@/theme';

const mockSeniors = jest.fn();

jest.mock('@/features/seniors/use-seniors', () => ({ useSeniors: () => mockSeniors() }));
jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ isSignedIn: true, isRestoring: false }),
}));
jest.mock('expo-router', () => ({ router: { push: jest.fn() }, Stack: { Screen: () => null } }));

function renderScreen() {
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
  return render(<CircleScreen />, { wrapper: Wrapper });
}

it('lists everyone you care for', () => {
  mockSeniors.mockReturnValue({
    data: [
      { id: '1', displayName: 'James Miller', isSelf: false, role: 'family_member' },
      { id: '2', displayName: 'Ada Lovelace', isSelf: true, role: 'family_member' },
    ],
    isPending: false,
    isError: false,
    refetch: jest.fn(),
  });
  renderScreen();

  expect(screen.getByRole('button', { name: 'James Miller, Family care' })).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Ada Lovelace, Your own care' })).toBeTruthy();
});

/**
 * An invited caregiver arrives with nobody, and so does a brand-new user. Both
 * need the two ways in, which is why they are on this screen and not buried.
 */
it('offers both ways to grow an empty circle', () => {
  mockSeniors.mockReturnValue({ data: [], isPending: false, isError: false, refetch: jest.fn() });
  renderScreen();

  expect(screen.getByRole('button', { name: 'Add another person' })).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Join a care circle' })).toBeTruthy();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec jest src/app/__tests__/circle-screen.test.tsx`
Expected: FAIL — `Cannot find module '../(tabs)/circle'`.

- [ ] **Step 3: Write minimal implementation**

Create `src/app/(tabs)/circle.tsx`:

```tsx
import type { Senior } from '@genxcare/contracts';
import { Stack, router } from 'expo-router';
import { ActivityIndicator, View } from 'react-native';

import { Button, Card, EmptyState, Screen, SummaryCard, Text } from '@/components/ui';
import { useSession } from '@/features/auth/session-provider';
import { useSeniors } from '@/features/seniors/use-seniors';
import { ApiError } from '@/lib/api-error';
import { useTheme } from '@/theme';

/**
 * Circle (docs/13, screen 10).
 *
 * Everyone you care for, in one place. A professional caregiver with six
 * clients gets a list they can work from; Today stays their own round rather
 * than six stacked dashboards.
 */
export default function CircleScreen() {
  const theme = useTheme();
  const { isSignedIn } = useSession();
  const seniors = useSeniors(isSignedIn);

  return (
    <Screen scrollable variant="list">
      <Stack.Screen options={{ title: 'Circle' }} />

      {seniors.isPending ? (
        <View style={{ alignItems: 'center', gap: theme.spacing.md, paddingVertical: theme.spacing.xxl }}>
          <ActivityIndicator color={theme.colors.primary} />
          <Text variant="secondary" color="secondary">
            Loading your care circle…
          </Text>
        </View>
      ) : seniors.isError ? (
        <Card>
          <Text variant="bodyStrong">We could not load your circle</Text>
          <Text variant="secondary" color="secondary">
            {seniors.error instanceof ApiError
              ? seniors.error.message
              : 'Something went wrong. Please try again.'}
          </Text>
          <Button variant="secondary" label="Try again" onPress={() => void seniors.refetch()} />
        </Card>
      ) : seniors.data.length === 0 ? (
        <EmptyState
          illustration="careCircle"
          title="Nobody here yet"
          body="Add the person you are caring for — or join a circle you have been invited to."
        />
      ) : (
        seniors.data.map((senior) => (
          <SummaryCard
            key={senior.id}
            name={senior.displayName}
            role={describeRole(senior)}
            href={{ pathname: '/seniors/[seniorId]', params: { seniorId: senior.id } }}
            stats={[]}
          />
        ))
      )}

      <View style={{ gap: theme.spacing.md, marginTop: theme.spacing.lg }}>
        <Button
          variant="secondary"
          label="Add another person"
          onPress={() => router.push('/onboarding')}
        />
        <Button
          variant="ghost"
          label="Join a care circle"
          onPress={() => router.push('/invitations/join')}
        />
      </View>
    </Screen>
  );
}

/** Plain-language description of the reader's relationship to this senior. */
function describeRole(senior: Senior): string {
  if (senior.isSelf) return 'Your own care';

  switch (senior.role) {
    case 'family_member':
      return 'Family care';
    case 'professional_caregiver':
      return 'In your professional care';
    default:
      return 'Care circle';
  }
}
```

> `stats={[]}` for now. Per-senior counts need a batched endpoint that does not exist; adding one is out of scope for this plan and belongs to phase 4. An empty array renders a card with no stat strip, which `SummaryCard` already handles and is covered by its test.

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm exec jest src/app/__tests__/circle-screen.test.tsx`
Expected: PASS, 2 tests.

- [ ] **Step 5: Commit**

```bash
git add "src/app/(tabs)/circle.tsx" src/app/__tests__/circle-screen.test.tsx
git commit -m "Add the Circle tab"
```

---

# Phase 3 — Today

### Task 12: Rebuild Today on the new components

**Files:**
- Modify: `src/app/(tabs)/home.tsx`
- Test: `src/app/__tests__/home-screen.test.tsx` (create)

**Interfaces:**
- Consumes: `SummaryCard`, `ListRow`, `SectionHeader`, `EmptyState`, `Screen`, `Text`, `useMyTasks()`, `useSeniors()`.
- Produces: route `/home`, unchanged.

- [ ] **Step 1: Write the failing test**

Create `src/app/__tests__/home-screen.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react-native';
import type { ReactNode } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import HomeScreen from '../(tabs)/home';
import { ThemeProvider } from '@/theme';

const mockTasks = jest.fn();

jest.mock('@/features/auth/session-provider', () => ({
  useSession: () => ({ isSignedIn: true, isRestoring: false }),
}));
jest.mock('@/features/seniors/use-seniors', () => ({
  useSeniors: () => ({ data: [], isPending: false, isError: false, refetch: jest.fn() }),
}));
jest.mock('@/features/tasks/use-tasks', () => ({ useMyTasks: () => mockTasks() }));
jest.mock('@/features/sync/use-sync', () => ({ useOfflineSync: jest.fn() }));
jest.mock('expo-router', () => ({
  router: { push: jest.fn() },
  Redirect: () => null,
  Stack: { Screen: () => null },
}));

function renderScreen() {
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
  return render(<HomeScreen />, { wrapper: Wrapper });
}

/** "Today" promised a day and showed a menu. It has to say which day it is. */
it("names today's date", () => {
  mockTasks.mockReturnValue({ data: [] });
  renderScreen();

  const today = new Date().toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'long' });
  expect(screen.getByText(today)).toBeTruthy();
});

it('lists your own round across every circle', () => {
  mockTasks.mockReturnValue({
    data: [{ id: 't1', seniorId: '1', title: 'Morning walk', status: 'pending', dueAt: null }],
  });
  renderScreen();

  expect(screen.getByText('Morning walk')).toBeTruthy();
});

/** Sign-out moved to Settings in task 10 and must not reappear here. */
it('does not offer sign out', () => {
  mockTasks.mockReturnValue({ data: [] });
  renderScreen();

  expect(screen.queryByRole('button', { name: 'Sign out' })).toBeNull();
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec jest src/app/__tests__/home-screen.test.tsx`
Expected: FAIL on the date assertion — Home renders no date.

- [ ] **Step 3: Rewrite Home**

Replace `src/app/(tabs)/home.tsx`. Keep the existing file-level doc comment, delete the `NotificationsButton`, `SeniorRow`, `describeRole` and `styles` definitions (Circle owns the people list now, and the alerts badge moved to the tab bar), and render:

```tsx
export default function HomeScreen() {
  const theme = useTheme();
  const { isSignedIn, isRestoring } = useSession();
  const seniors = useSeniors(isSignedIn);
  const myTasks = useMyTasks();

  useOfflineSync();

  if (!isRestoring && !isSignedIn) return <Redirect href="/sign-in" />;

  const tasks = myTasks.data ?? [];
  const people = seniors.data ?? [];

  return (
    <Screen scrollable variant="list">
      <Stack.Screen options={{ headerShown: false }} />

      <View style={{ gap: theme.spacing.xs, marginBottom: theme.spacing.sm }}>
        <Text variant="label" color="secondary">
          {new Date().toLocaleDateString(undefined, {
            weekday: 'long',
            day: 'numeric',
            month: 'long',
          })}
        </Text>
        <Text accessibilityRole="header" variant="pageHeading">
          Today
        </Text>
      </View>

      {tasks.length === 0 ? (
        <EmptyState
          illustration="allCaughtUp"
          title="Nothing needs you"
          body="When something is due for anyone in your circle, it will appear here."
        />
      ) : (
        <>
          <SectionHeader title="Yours to do" />
          {tasks.slice(0, 5).map((task) => (
            <ListRow
              key={task.id}
              title={task.title}
              subtitle={`${taskTimeLabel(task, timezoneFor(people, task.seniorId))} · ${statusLabel(task.status)}`}
              href={{ pathname: '/tasks/[taskId]', params: { taskId: task.id } }}
            />
          ))}
        </>
      )}
    </Screen>
  );
}

/** This list spans circles, so each row is read in its own senior's timezone. */
function timezoneFor(people: Senior[], seniorId: string): string {
  return people.find((person) => person.id === seniorId)?.timezone ?? 'UTC';
}
```

Imports needed: `statusLabel`, `taskTimeLabel` and `type Senior` from `@genxcare/contracts`; `Redirect`, `Stack` from `expo-router`; `View` from `react-native`; `EmptyState`, `ListRow`, `Screen`, `SectionHeader`, `Text` from `@/components/ui`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm exec jest src/app && pnpm typecheck`
Expected: PASS.

- [ ] **Step 5: Run every gate**

Run: `pnpm test && pnpm typecheck && pnpm lint && pnpm exec prettier --check "src/**/*.{ts,tsx}"`
Expected: all pass.

- [ ] **Step 6: Drive the running app**

Not optional. A screen that passes tests and looks wrong is not done.

```bash
pnpm ios
```

Then, with the simulator booted:

```bash
xcrun simctl io booted screenshot /tmp/today.png && sips -Z 700 /tmp/today.png
```

Look at the screenshot. Confirm: the tab bar has four tabs with icons; Today shows a date and either a task list or the empty state; no sign-out button; tapping Settings reaches sign-out. Repeat with the simulator in dark mode (Settings → Developer → Dark Appearance) and confirm the ground is neutral grey, not teal-tinted.

- [ ] **Step 7: Commit**

```bash
git add "src/app/(tabs)/home.tsx" src/app/__tests__/home-screen.test.tsx
git commit -m "Make Today about today"
```

---

## Self-Review

**Spec coverage.** Dark ramp, `elevation`, `primarySubtle`, `label` → task 1. Icons → task 2. `ListRow`, `Avatar`, `StatChip`, `SectionHeader`, `SummaryCard`, `EmptyState`, `Screen` variants → tasks 3–7. Header defaults → task 8. `(tabs)` group and preserved URLs → task 9. Settings, Profile, sign-out relocation → task 10. Circle → task 11. Today → task 12. Defect 1 (unstyled row) → task 3 by construction, removed from Home in task 12.

**Known gaps, deliberate.**
- The spec's nine magic spacing numbers are fixed only in files these tasks touch (`home.tsx`, `button.tsx` untouched). The rest go with phase 4.
- `docs/18` and `docs/13` are not rewritten here. The spec says they are updated once phase 2 lands; that is a documentation task to run after task 11, not a code task.
- Per-senior stats on Circle are stubbed to `[]` and flagged inline. They need an endpoint that does not exist.

**Type consistency.** `IconName` (task 2) is consumed by tasks 3, 6, 9. `StatTone` (task 5) is consumed by task 6. `SummaryStat` (task 6) is consumed by task 11. `ScreenVariant` (task 7) is consumed by tasks 10, 11, 12. `initials` is exported for its own test only. All names match across tasks.
