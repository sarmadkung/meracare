import type { Theme } from './theme-provider';

/**
 * Header styling, derived from the theme.
 *
 * Nested navigators do not inherit `screenOptions` from the stack above them,
 * so every layout that shows a header has to say this — and saying it by hand
 * is how the Settings header ended up default white against an off-white
 * screen, with a seam across the top of the page.
 *
 * Screens still declare only their title.
 */
export function headerOptions(theme: Theme) {
  return {
    headerStyle: { backgroundColor: theme.colors.background },
    headerTintColor: theme.colors.primary,
    headerTitleStyle: {
      color: theme.colors.textPrimary,
      fontFamily: 'Inter_600SemiBold',
      fontSize: 17,
    },
    headerShadowVisible: false,
    contentStyle: { backgroundColor: theme.colors.background },
  } as const;
}
