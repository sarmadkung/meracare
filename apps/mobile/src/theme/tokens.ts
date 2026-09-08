/**
 * MeraCare design tokens.
 *
 * The locked visual system lives in `docs/18-visual-theme-and-illustrations.md`:
 * green with a slight blue/teal bias, `#0F766E` Deep Teal as the brand colour,
 * light mode primary and dark mode preserving the same semantic identity.
 *
 * Components must consume the semantic names below (`colors.primary`,
 * `colors.danger`, …) and never hardcode a hex value.
 */

/** Raw palette. Only the theme definitions below may reference these. */
const palette = {
  teal900: '#134E4A',
  teal800: '#115E59',
  teal700: '#0F766E',
  teal500: '#14B8A6',
  teal300: '#5EEAD4',
  teal100: '#CCFBF1',

  // Dark mode is Neutral Charcoal: a hueless ramp. A teal-tinted ground made
  // dark mode read as a different product from light (docs/superpowers/specs/
  // 2026-09-08-mobile-design-system-and-navigation-design.md).
  zinc950: '#131416',
  zinc900: '#1D1E21',
  zinc800: '#27282C',
  zinc700: '#2E2F33',
  zinc500: '#71717A',
  zinc400: '#A1A1AA',
  zinc50: '#F4F4F5',

  /** A quiet brand fill for large areas. `teal100` is too saturated to sit under text. */
  tealSubtleLight: '#F0FDF9',
  tealSubtleDark: '#14312E',

  /** Light mode's body text. Kept from the original ramp: a near-black with a green cast. */
  slate800: '#172B2A',
  slate500: '#64748B',
  slate400: '#94A3B8',
  slate200: '#E2E8E8',
  slate100: '#EEF2F2',
  slate50: '#F8FAFA',
  white: '#FFFFFF',

  green700: '#15803D',
  green400: '#4ADE80',
  green100: '#DCFCE7',

  amber700: '#B45309',
  amber400: '#FBBF24',
  amber100: '#FEF3C7',

  red700: '#B91C1C',
  red400: '#F87171',
  red100: '#FEE2E2',
} as const;

/** Semantic colour roles. Both themes must define every key. */
export interface ThemeColors {
  primary: string;
  primaryDark: string;
  primaryLight: string;
  /** A quiet brand tint for a large fill, such as the ground behind a stat. */
  primarySubtle: string;
  /** Text and icons drawn on top of `primary`. */
  onPrimary: string;
  teal: string;
  mint: string;

  background: string;
  surface: string;
  /** A recessed surface, e.g. an inset row inside a card. */
  surfaceMuted: string;

  textPrimary: string;
  textSecondary: string;
  /** Text on a disabled control or a placeholder. */
  textMuted: string;
  border: string;

  success: string;
  successBackground: string;
  warning: string;
  warningBackground: string;
  danger: string;
  dangerBackground: string;
}

/** Light mode is the primary mode (docs/18). */
export const lightColors: ThemeColors = {
  primary: palette.teal700,
  primaryDark: palette.teal900,
  primaryLight: palette.teal100,
  primarySubtle: palette.tealSubtleLight,
  onPrimary: palette.white,
  teal: palette.teal500,
  mint: palette.teal300,

  background: palette.slate50,
  surface: palette.white,
  surfaceMuted: palette.slate100,

  textPrimary: palette.slate800,
  textSecondary: palette.slate500,
  textMuted: palette.slate400,
  border: palette.slate200,

  success: palette.green700,
  successBackground: palette.green100,
  warning: palette.amber700,
  warningBackground: palette.amber100,
  danger: palette.red700,
  dangerBackground: palette.red100,
};

/**
 * Dark mode keeps the same semantic identity: teal is still the brand, green
 * still means done, amber still means attention, red still means critical.
 */
export const darkColors: ThemeColors = {
  primary: palette.teal500,
  primaryDark: palette.teal700,
  primaryLight: palette.teal900,
  primarySubtle: palette.tealSubtleDark,
  onPrimary: palette.zinc950,
  teal: palette.teal500,
  mint: palette.teal300,

  background: palette.zinc950,
  surface: palette.zinc900,
  surfaceMuted: palette.zinc800,

  textPrimary: palette.zinc50,
  textSecondary: palette.zinc400,
  textMuted: palette.zinc500,
  border: palette.zinc700,

  success: palette.green400,
  successBackground: '#123524',
  warning: palette.amber400,
  warningBackground: '#3B2A0B',
  danger: palette.red400,
  dangerBackground: '#3F1414',
};

/** 4pt spacing scale. */
export const spacing = {
  xs: 4,
  sm: 8,
  md: 12,
  lg: 16,
  xl: 24,
  xxl: 32,
  xxxl: 48,
} as const;

export const radii = {
  sm: 8,
  md: 12,
  lg: 16,
  pill: 999,
} as const;

/**
 * Type scale from docs/18, sized for older adults: body starts at 17pt and
 * primary actions are never smaller than 17pt.
 */
export const typography = {
  pageHeading: { fontFamily: 'Inter_700Bold', fontSize: 30, lineHeight: 38 },
  sectionHeading: { fontFamily: 'Inter_600SemiBold', fontSize: 23, lineHeight: 30 },
  body: { fontFamily: 'Inter_400Regular', fontSize: 17, lineHeight: 26 },
  bodyStrong: { fontFamily: 'Inter_600SemiBold', fontSize: 17, lineHeight: 26 },
  secondary: { fontFamily: 'Inter_400Regular', fontSize: 15, lineHeight: 22 },
  action: { fontFamily: 'Inter_600SemiBold', fontSize: 18, lineHeight: 24 },
  /**
   * The eyebrow above a section or beside a number. Small and letter-spaced, so
   * it reads as a label rather than as something to be read in sequence. Never
   * used for anything a person has to actually read.
   */
  label: { fontFamily: 'Inter_600SemiBold', fontSize: 13, lineHeight: 16, letterSpacing: 0.6 },
} as const;

export type TypographyVariant = keyof typeof typography;

/**
 * Minimum interactive size. docs/18 requires at least 48dp touch targets.
 */
export const minTouchTarget = 48;

/**
 * Named shadow presets.
 *
 * iOS reads the `shadow*` properties and Android reads `elevation`, so every
 * preset sets both. Screens must never write a shadow inline: an app whose
 * cards lift by different amounts looks unfinished even when nothing else is
 * wrong, and that is exactly how the current screens drifted apart.
 *
 * The shadow colour is a literal rather than a palette role because a shadow is
 * not a surface — it is the absence of light, and it stays the same dark
 * regardless of theme. Only its opacity does the work.
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
