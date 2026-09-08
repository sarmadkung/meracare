# Mobile design system and navigation

**Date:** 2026-09-08
**Status:** Draft — awaiting review
**Supersedes:** parts of `docs/18-visual-theme-and-illustrations.md` (dark palette) and
`docs/13-mvp-screen-map.md` (navigation model)

## Why

The app works and does not look designed. Three complaints, one root cause each:

1. **"There is no navigation."** There is no tab bar. Every top-level destination —
   Today, the people you care for, alerts, settings — is reached by tapping a
   full-width button stacked on Home.
2. **"Sign out is in the middle of the home screen."** `docs/13` specifies screens
   24 (Profile) and 26 (Account/settings). Neither was built, so sign-out had
   nowhere to live and landed on Home between "Notification settings" and empty
   space.
3. **"The user name is broken design."** A real bug, not taste. See
   [Defects](#defects-this-fixes).

A fourth, unstated: the app has **no icons at all**. The chevron on a senior row is
the literal text character `›`. Nothing else is iconographic. This is most of why
the app reads as a prototype.

### What is not wrong

Token discipline is already strong, and the redesign must not weaken it:

- Zero hardcoded `fontSize` outside `src/theme/`.
- Colours hardcoded in exactly two places — `apple-button.tsx` and
  `google-button.tsx` — where Apple and Google brand guidelines *require* exact
  values. These stay.
- Nine stray spacing numbers across 30 screens.

Inconsistency is at the **composition** layer, not the token layer: some screens
wrap content in `Card`, some in raw `View`; `Screen` applies one padding to forms
and lists alike; 24 screens each re-declare their own `Stack.Screen` options
instead of inheriting defaults, and `settings/notifications.tsx` repeats an
identical `Stack.Screen` three times across three render branches.

Correcting an earlier misreading, recorded so the plan is not built on it: headers
are *not* missing. 24 of 30 screens set `headerShown: true` with a real title.
Only the senior dashboard uses `title: ''`. The header problem is that Home has
none, styling is unthemed, and the declaration is duplicated per screen rather
than defaulted in a layout.

## Decisions

Chosen by the user against rendered alternatives on 2026-09-08.

| Decision | Choice |
|---|---|
| Visual direction | **Calm Clinical** — white cards, hairline borders, soft shadows, generous air. Teal stays the brand. |
| Dark mode | **Neutral Charcoal** — colourless grey ramp; teal appears only where it carries meaning. |
| Navigation | **Four tabs** — Today · Circle · Alerts · Settings |

Four tabs was chosen over three because `docs/13` already lists Home/Today (9) and
Seniors (10) as separate screens, and explicitly forbids splitting into separate
"Family App" and "Professional App" architectures. Four tabs gives a professional
caregiver a real client list without a second app.

## Tokens

`src/theme/tokens.ts` is the only file that changes for colour.

### Dark palette — replaced

The current dark ramp is teal-tinted (`slate950: #0B1A19` … `slate700: #1F3634`).
Neutral Charcoal replaces it with a hueless ramp. The teal-tinted values are
deleted, not kept alongside, so there is one dark ground and no drift.

| Role | Was | Becomes |
|---|---|---|
| `background` | `#0B1A19` | `#131416` |
| `surface` | `#12211F` | `#1D1E21` |
| `surfaceMuted` | `#172B2A` | `#27282C` |
| `border` | `#1F3634` | `#2E2F33` |
| `textPrimary` | `#F8FAFA` | `#F4F4F5` |
| `textSecondary` | `#94A3B8` | `#A1A1AA` |
| `textMuted` | `#64748B` | `#71717A` |

`primary` stays `#14B8A6` in dark and `#0F766E` in light. This asymmetry is
deliberate and already correct: the light teal fails contrast on a dark ground.
"The same colour throughout" means the same *semantic role*, not the same hex.

Light mode is unchanged apart from two additions below.

### Added tokens

Calm Clinical needs three things the current set cannot express:

- **`elevation`** — named shadow presets (`none`, `card`, `raised`). Today every
  shadow is either absent or written inline. Cross-platform: iOS shadow props plus
  Android `elevation`.
- **`colors.primarySubtle`** — the tinted fill behind a stat chip (`#F0FDF9` light,
  `#14312E` dark). `primaryLight` (`#CCFBF1`) is too saturated for a large fill and
  is what makes the current secondary buttons read as mint slabs.
- **`typography.label`** — 11pt semibold, letter-spaced, for the `UP NEXT` /
  `MONDAY, 8 SEPT` eyebrow. Sits below `secondary` in the scale.

### Accessibility floor — unchanged

Non-negotiable, and every new component inherits it:

- Body text 17pt minimum, primary actions 18pt.
- Touch targets 48dp minimum (`minTouchTarget`).
- Colour is never the only carrier of meaning; status always has a text label.

The mockups were drawn small to fit three phones on a screen. They are **not** the
implementation sizes.

## Icons

`@expo/vector-icons` is not installed and must be added. It ships bundled fonts,
needs no native build, and works in Expo Go. `react-native-svg` is not needed —
illustrations are PNGs loaded through `require`.

Screens never import from `@expo/vector-icons` directly. A single
`components/ui/icon.tsx` maps semantic names (`chevron`, `bell`, `pill`, `calendar`,
`people`, `settings`, `today`) to glyphs, so the icon set can be swapped in one
file and no screen can invent a one-off glyph. This is the same containment
`Illustration` already uses for unDraw assets — follow that file's shape.

## Navigation

### Route tree

Groups in expo-router do not appear in the URL, so `app/(tabs)/home.tsx` still
serves `/home`. **Every existing route keeps its current URL.** This matters: there
are 8 references to `/home`, 4 to `/notifications`, and 1 to
`/settings/notifications` in source, plus notification deep links built at runtime
in `use-reminder-sync.ts` and external invitation links.

```
app/
  _layout.tsx                    Stack, themed defaults  (edited)
  index.tsx                      session gate → /home or /sign-in  (unchanged)
  sign-in.tsx  onboarding.tsx  auth/callback.tsx        (outside tabs)
  invitations/…                                          (outside tabs)

  (tabs)/
    _layout.tsx                  Tabs                    (new)
    home.tsx                     /home           Today   (moved)
    circle.tsx                   /circle         Circle  (new)
    notifications.tsx            /notifications  Alerts  (moved)
    settings/
      _layout.tsx                Stack                   (new)
      index.tsx                  /settings               (new)
      profile.tsx                /settings/profile       (new)
      notifications.tsx          /settings/notifications (moved)

  seniors/…  tasks/…  medications/…  appointments/…      (unchanged paths)
```

Detail routes stay **outside** the tabs group and push full-screen over the tab
bar. They are reached from both Today and Circle; nesting them inside one tab
would either duplicate the route tree or make a senior opened from Today appear
under the wrong tab.

`Tabs` must be imported from **`expo-router/js-tabs`** — `import { Tabs } from
'expo-router'` is deprecated in expo-router 57. No dependency is needed:
expo-router bundles its own `bottom-tabs` fork internally. `expo-router/unstable-native-tabs`
exists and gives a genuinely native iOS tab bar, but is explicitly unstable and is not used here.

### Tab contents

- **Today** — the date, your own round across every circle, what is next, what is
  due. Time-oriented. This is what "Today" already promises and does not deliver.
- **Circle** — every person you care for, as cards with a status summary. Plus
  "Add another person" and "Join a care circle", which are the two actions
  currently marooned on Home.
- **Alerts** — the existing inbox, unchanged in behaviour, with its unread badge
  moved onto the tab icon.
- **Settings** — Profile, Notification settings, and **Sign out**.

### Headers

Themed header defaults move into `app/_layout.tsx` `screenOptions` — background,
tint, title font, back-button behaviour — so a screen declares only its title.
The per-branch duplication in `settings/notifications.tsx` collapses to one
declaration.

## Component layer

This is the mechanism that makes composition consistent. Consistency comes from
shared components, not from repeating the same effort on 30 screens.

| Component | Purpose | Replaces |
|---|---|---|
| `Icon` | semantic name → glyph | the literal `›` text character |
| `ListRow` | tappable row: leading slot, title, subtitle, trailing chevron | the broken `Link asChild` + `Pressable` pattern |
| `Avatar` | initials or photo, themed | nothing — new |
| `SummaryCard` | person card with header row and stat strip | the unstyled senior row |
| `StatChip` | one number + label on a tinted ground | nothing — new |
| `SectionHeader` | the `UP NEXT` eyebrow | ad-hoc `<Text variant="secondary">` |
| `EmptyState` | illustration + heading + body + action | duplicated inline in several screens |

`Screen` gains a `variant` prop (`list` | `form` | `detail`) so padding and gap
stop being uniform across content types that need different rhythm. Its current
behaviour becomes `variant="form"` so existing screens are unaffected until
migrated.

Every component is tested at the boundary a user meets: role, accessible name, and
what appears — never internal structure.

## Defects this fixes

**1. The unstyled senior row — the "broken user name".**
`home.tsx` wraps a styled `Pressable` in `<Link … asChild>`. expo-router clones the
child and passes its own `style` through, so the entire style array is discarded:
no card, no background, no padding, no `flexDirection: 'row'` — which is why the
`›` falls onto its own line. `asChild` is still a supported prop; the collision is
with `style` specifically. Both occurrences are in `home.tsx` (lines 217 and 262)
and they are the only two `asChild` uses in the app. `ListRow` owns navigation
internally via `router.push`, so the pattern cannot recur.

**2. Nine magic spacing numbers** in `home.tsx`, `settings/notifications.tsx`,
`seniors/[seniorId]/index.tsx`, `notification-row.tsx`, `google-button.tsx` and
`button.tsx` — replaced with tokens as their files are touched, not as a separate
sweep.

## Testing

- Every new component gets tests written first, watched fail, per the project's
  TDD practice.
- The suite is 43 files / 343 tests, all passing, and must stay green. Screen
  tests that assert on layout internals will need updating as screens migrate;
  tests that assert on user-visible behaviour should not.
- `home-screen.test.tsx` already covers the sign-out error path and must keep
  passing after Home moves into `(tabs)` and sign-out moves to Settings — the
  assertion moves with the button.
- Navigation gets one smoke test: the four tabs render with correct accessible
  names, and each routes to its screen.
- Gates per phase: `pnpm test`, `tsc --noEmit`, `expo lint`, `prettier --check`.
- Every phase ends by driving the running app in the simulator and screenshotting
  it. A component that passes tests and looks wrong is not done.

## Phasing

Thirty screens is too much for one change. Each phase lands independently, keeps
the app shippable, and is reviewable on its own.

1. **Foundation** — tokens (dark ramp, elevation, `primarySubtle`, `label`), the
   icon library and `Icon`, `ListRow`, `Avatar`, `StatChip`, `SectionHeader`,
   `SummaryCard`, `EmptyState`. No screen changes. Fixes defect 1 by construction.
2. **Navigation** — the `(tabs)` group, the four tabs, themed header defaults, the
   Settings stack with Profile, and sign-out relocated. Deep links verified.
3. **Today and Circle** — the two screens the user sees first, rebuilt on the new
   components to Calm Clinical.
4. **The remaining screens** — senior dashboard, then the domain screens, migrated
   onto the component layer.

Phases 1–3 answer every complaint that started this. Phase 4 is the long tail and
can be paused after any screen.

## Out of scope

- The API, data model, and permissions are untouched.
- No new features. Screens gain no capability they did not have.
- Illustration assets stay as they are; only their framing changes.
- `docs/18` and `docs/13` are updated to match this spec once phase 2 lands, not
  before — they should describe what exists.
