import { darkColors, elevation, lightColors, minTouchTarget, typography } from '../tokens';

describe('theme tokens', () => {
  it('uses the locked Deep Teal brand colour in light mode', () => {
    expect(lightColors.primary).toBe('#0F766E');
  });

  it('defines every semantic role in both themes', () => {
    expect(Object.keys(darkColors).sort()).toEqual(Object.keys(lightColors).sort());

    for (const [role, value] of Object.entries(darkColors)) {
      expect(value).toMatch(/^#[0-9A-Fa-f]{6}$/);
      expect(lightColors[role as keyof typeof lightColors]).toMatch(/^#[0-9A-Fa-f]{6}$/);
    }
  });

  it('keeps touch targets at the 48dp accessibility minimum', () => {
    expect(minTouchTarget).toBeGreaterThanOrEqual(48);
  });

  it('keeps body and action text large enough for older adults', () => {
    expect(typography.body.fontSize).toBeGreaterThanOrEqual(16);
    expect(typography.action.fontSize).toBeGreaterThanOrEqual(16);
    expect(typography.pageHeading.fontSize).toBeGreaterThanOrEqual(28);
  });
});

describe('Neutral Charcoal dark mode', () => {
  /**
   * Dark mode is a hueless ground. The previous ramp was teal-tinted, which
   * made dark mode read as a different product from light rather than the same
   * one after dusk.
   */
  it('uses a hueless ground', () => {
    // A neutral grey has equal-ish R, G and B. The old #0B1A19 did not.
    const channels = [1, 3, 5].map((i) => parseInt(darkColors.background.slice(i, i + 2), 16));
    expect(Math.max(...channels) - Math.min(...channels)).toBeLessThanOrEqual(4);
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
});

describe('composition tokens', () => {
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
});
