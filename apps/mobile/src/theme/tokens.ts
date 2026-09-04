/**
 * GenXcare design tokens.
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

  slate950: '#0B1A19',
  slate900: '#12211F',
  slate800: '#172B2A',
  slate700: '#1F3634',
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
  onPrimary: palette.slate950,
  teal: palette.teal500,
  mint: palette.teal300,

  background: palette.slate950,
  surface: palette.slate900,
  surfaceMuted: palette.slate800,

  textPrimary: palette.slate50,
  textSecondary: palette.slate400,
  textMuted: palette.slate500,
  border: palette.slate700,

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
  pageHeading: { fontSize: 30, lineHeight: 38, fontWeight: '700' },
  sectionHeading: { fontSize: 23, lineHeight: 30, fontWeight: '600' },
  body: { fontSize: 17, lineHeight: 26, fontWeight: '400' },
  bodyStrong: { fontSize: 17, lineHeight: 26, fontWeight: '600' },
  secondary: { fontSize: 15, lineHeight: 22, fontWeight: '400' },
  action: { fontSize: 18, lineHeight: 24, fontWeight: '600' },
} as const;

export type TypographyVariant = keyof typeof typography;

/**
 * Minimum interactive size. docs/18 requires at least 48dp touch targets.
 */
export const minTouchTarget = 48;
