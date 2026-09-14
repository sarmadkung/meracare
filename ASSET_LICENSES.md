# Asset Licenses

## Inter

- Source: [The Inter typeface project](https://github.com/rsms/inter)
- Distribution: `@expo-google-fonts/inter` 0.4.2
- License: SIL Open Font License 1.1
- Usage: application typography

The package includes the complete font license in its `LICENSE_FONT` file. No
font files have been modified or renamed.

## Google G mark

- File: `apps/mobile/assets/images/google-g.png`
- Usage: Google authentication button
- Source and use must continue to follow Google's identity guidelines.

## Application artwork

- Approved source mark: `apps/mobile/assets/images/brand-mark.png`
- Created: 2026-08-24 with OpenAI image generation
- Prompt direction: a Deep Teal family-care emblem with two people, protective
  hands, a heart, no text, and a transparent background
- Approved by product: 2026-08-24
- Runtime exports: `icon.png`, `android-icon-foreground.png`,
  `android-icon-background.png`, `android-icon-monochrome.png`,
  `splash-icon.png`, and `favicon.png`

The runtime exports were derived deterministically from the approved source. The
alternative `brand-mark-v2.png` is retained as an unselected concept for design
history and is not referenced by the application. Store artwork and a social
preview still require final export and review.

## unDraw illustrations

- Creator: Katerina Limpitsouni / unDraw
- License: [unDraw license](https://undraw.co/license), reviewed 2026-08-24
- Usage: bundled GenXcare onboarding and empty-state artwork
- Modifications: the primary `#6C63FF` accent was changed to GenXcare Deep Teal
  `#0F766E`; transparent PNG runtime exports were rendered from the retained SVG
  sources
- Attribution: not required by the license; provenance is recorded here

| Local asset     | Original illustration | Source                                                 |
| --------------- | --------------------- | ------------------------------------------------------ |
| `welcome-team`  | Team                  | https://undraw.co/illustration/team_85hs               |
| `add-senior`    | Add user              | https://undraw.co/illustration/add-user_rbko           |
| `all-caught-up` | Complete task         | https://undraw.co/illustration/complete-task_qgwk      |
| `care-circle`   | Team collaboration    | https://undraw.co/illustration/team-collaboration_phnf |
| `communication` | Chat                  | https://undraw.co/illustration/chat_qmyo               |

The files live under `apps/mobile/assets/illustrations/undraw/`. They are part of
GenXcare's interface and must not be redistributed as an illustration pack or
used for AI/ML training. Recheck the linked license before release.
