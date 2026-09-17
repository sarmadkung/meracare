# @genxcare/marketing

The GenxCare marketing site. One page: what the app does, who it is for, and the
App Store / Google Play links.

Plain HTML and CSS with no build step and no dependencies, so it deploys as
static files to Vercel, Netlify, Cloudflare Pages, or GitHub Pages with the
output directory set to `apps/marketing`.

## Run it locally

```bash
pnpm --filter @genxcare/marketing dev   # http://localhost:4173
```

## Structure

```text
index.html            the whole page, including the inline SVG illustrations
styles.css            design tokens and layout
assets/favicon.svg    brand mark
```

## Design

The palette and type scale follow `docs/18-visual-theme-and-illustrations.md`:
Deep Teal `#0F766E`, mint accents, neutral surfaces, and status colour used only
for status. Fraunces is the display face, Inter the body face — the same face
the app uses.

The illustrations are drawn inline as SVG in the brand palette rather than taken
from unDraw or Storyset. Nothing is hotlinked, nothing needs an attribution
entry, and the recolouring problem the docs warn about disappears. The signature
element is the care circle in the hero: one person at the centre, her circle
orbiting, and the two things the circle actually shares floating alongside.

Motion is a load-in stagger, a 48-second orbit, and a slow float. All of it is
disabled under `prefers-reduced-motion`.

## Before this goes live

- [ ] Replace the placeholder store URLs. They appear in three places in
      `index.html`: the hero, the closing section, and the footer. Currently
      `id0000000000` (Apple) and `app.genxcare.mobile` is already correct, taken from apps/mobile/app.json.
- [ ] Swap the in-brand download buttons for Apple's and Google's official
      badge artwork — both stores require their own badge assets.
- [ ] Add a privacy policy page and link it in the footer. Both stores require a
      reachable privacy policy URL before submission.
- [ ] Add `assets/social-card.png` (1200×630) for the Open Graph preview, or
      remove the `og:image` and `twitter:card` tags.
- [ ] Confirm the `hello@genxcare.app` support address and the `genxcare.app`
      canonical domain.
