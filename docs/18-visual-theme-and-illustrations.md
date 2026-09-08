# Senior Care --- Visual Theme, Color Palette, and Illustration System

## Status

**Accepted / Locked for MVP**

This is the approved visual direction for the Senior Care MVP and future
web application.

## Design Direction

The visual identity should feel:

-   calm
-   trustworthy
-   warm
-   modern
-   accessible
-   dignified
-   human

Avoid a hospital-administration aesthetic, childish visuals, or a
generic corporate SaaS appearance.

The visual identity should communicate:

> Care + Trust + Independence + Family

## Color Palette

The approved color direction is:

> **Green with a slight blue/teal bias.**

### Primary Brand Color

**Deep Teal --- `#0F766E`**

This is the main brand and interactive color.

### Supporting Palette

  Token                 Hex         Purpose
  --------------------- ----------- -----------------------------
  `primary`             `#0F766E`   Main brand/action color
  `primaryDark`         `#134E4A`   Pressed/dark primary
  `primaryLight`        `#CCFBF1`   Soft primary background
  `teal`                `#14B8A6`   Secondary accent
  `mint`                `#5EEAD4`   Highlight/decorative accent
  `background`          `#F8FAFA`   Main application background
  `surface`             `#FFFFFF`   Cards and surfaces
  `textPrimary`         `#172B2A`   Primary text
  `textSecondary`       `#64748B`   Secondary text
  `border`              `#E2E8E8`   Borders/dividers
  `success`             `#15803D`   Completed/success state
  `successBackground`   `#DCFCE7`   Success surface
  `warning`             `#B45309`   Attention state
  `warningBackground`   `#FEF3C7`   Warning surface
  `danger`              `#B91C1C`   Critical/destructive state
  `dangerBackground`    `#FEE2E2`   Danger surface

Implement these as semantic design tokens. Components should not
hardcode raw colors.

### Color Rules

-   Teal is the brand color.
-   Green status is reserved for successful/completed states.
-   Amber indicates attention.
-   Red indicates critical/destructive states.
-   Never communicate important information through color alone.
-   Maintain strong contrast.
-   Avoid excessive saturation.
-   Light mode is the primary mode.
-   Dark mode should preserve the same semantic identity.

## Typography

Primary typography direction:

**Inter**

Recommended starting scale:

-   Page heading: 28--32
-   Section heading: 22--26
-   Body: 16--18
-   Secondary body: 15--16
-   Important senior-facing actions: 16--18+

The exact platform font implementation can vary between React Native and
web while retaining the same typographic scale.

The Expo application bundles Inter 400, 600, and 700 through
`@expo-google-fonts/inter` and maps the semantic regular, semibold, and bold
tokens to those faces. Font and candidate brand provenance is recorded in
`ASSET_LICENSES.md`.

## Accessibility

The visual system is specifically designed for older adults.

Requirements:

-   Target at least 48×48dp/pt touch targets.
-   Strong text contrast.
-   Large readable body text.
-   Clear pressed/focused states.
-   Do not rely on color alone.
-   Support platform text scaling where practical.
-   Avoid dense information layouts.
-   Use simple language.
-   Make primary actions obvious.

## Illustration System

The MVP will use two approved online illustration sources:

### Primary --- unDraw

Official source:

https://undraw.co/

Use unDraw primarily for:

-   empty states
-   onboarding
-   simple care concepts
-   success states
-   informational screens
-   lightweight decorative illustrations

Prefer SVG assets and recolor them toward the Senior Care palette where
the source permits it.

### Secondary --- Storyset

Official source:

https://storyset.com/

Use Storyset primarily for:

-   onboarding scenes
-   family/caregiver scenes
-   professional caregiver scenes
-   richer empty states
-   feature introductions
-   marketing/illustrative moments

Prefer one consistent Storyset illustration style within a feature or
flow.

## Illustration Rules

-   Do not mix unrelated illustration styles on the same screen.
-   Prefer the Senior Care teal/green palette where customization is
    available.
-   Warm supporting colors can be used sparingly.
-   Do not use illustrations that portray seniors as childish, helpless,
    or stereotypical.
-   Illustrations should communicate independence, capability, family
    connection, and dignity.
-   Do not hotlink third-party illustration assets in production.
-   Record source, asset name, URL, and license/attribution requirements
    for every shipped asset.
-   Maintain an `ASSET_LICENSES.md` file once production assets are
    selected.
-   `apps/mobile/assets/images/brand-mark.png` is the product-approved GenXcare
    mark. Runtime icon, adaptive-icon, monochrome, splash, and favicon exports
    derive from it. `brand-mark-v2.png` is an unselected concept and is not used
    by the application. Store and social-card exports still require final review.
-   Review the current license for each asset before shipping. Licenses
    can change independently of our product documentation.

Suggested asset structure:

``` text
assets/
  illustrations/
    undraw/
    storyset/
```

The implemented MVP set uses five locally bundled unDraw illustrations:
`welcome-team`, `add-senior`, `all-caught-up`, `care-circle`, and
`communication`. Screens reference semantic names through the shared
`Illustration` component rather than vendor filenames. The component provides a
consistent restrained frame and keeps artwork decorative by default so nearby
headings and instructions are not announced twice by assistive technology.

## Illustration Placement

### Onboarding

Large illustration is encouraged.

### Empty states

Small or medium illustration.

### Success states

Small illustration when it adds warmth.

### Main dashboards

Information comes first. Avoid large illustrations competing with care
data.

### Error states

Use illustrations only when they improve comprehension.

## Platform Consistency

The visual system must work consistently across:

-   iOS
-   Android
-   future web

Use semantic design tokens so the same palette can be implemented in
React Native and future Next.js web.

## Final Decision

The MVP visual identity is:

> **Deep Teal + Mint + Neutral surfaces + restrained semantic status
> colors + accessible typography + unDraw + Storyset illustrations.**

This decision is stable for MVP.
