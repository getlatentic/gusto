# Styling the app

All styling is Tailwind v4. There is no other stylesheet for the app besides the runtime tokens.

| File | Job |
| --- | --- |
| `static/tokens.css` | The `--gu-*` colour, type and spacing tokens, switched by `data-theme` and `data-accent`. |
| `theme.css` | Maps the tokens to Tailwind names (`bg-canvas`, `text-ink`, `border-line`, `rounded-md`, `shadow-1`). |
| `base.css` | Element defaults: body, headings, tables, focus ring, `svg.i`. |
| `prose.css` | Rendered Markdown. |
| `basecoat-theme.css`, `vendor/basecoat/` | [Basecoat](https://basecoatui.com) (MIT): shadcn's design system as plain HTML classes, without React. Its colour names are gusto's tokens. Use its `input`, `select`, `textarea`, `checkbox` and radio classes; `vendor/basecoat/update.sh` refreshes it. |
| `../templates/ui.html` | The one set of shared pieces: `ui-btn`, `ui-card`, `ui-field`, `ui-badge`, `ui-table`, `ui-crumb` and the rest, as class partials, and `ui-state` and `ui-chev` as markup. A page uses these and does not define its own. |

Rules:

- Write utilities in the template. Reach for a component class only when five or more places repeat the same pile.
- Use token-backed names only (`bg-surface`, `text-muted`, `border-line`). No hex, no arbitrary colours.
- Layout adapts to the page width with container variants: `@max-[56rem]:` (phone and narrow panes), never `max-*:` viewport variants.
- A disclosure is `<details class="group">` with a `ui-summary` summary and a `ui-chev` chevron. Never style the default marker.
- Class names and data attributes the scripts use stay in the markup: `.scrim`, `#side`, `[data-side-toggle]`, `[data-panel-toggle]`, `[data-look]`, `.picker.open`, and so on. A script that builds markup builds it with the same utilities.
- Sizes are rem. The build rewrites `px` to `rem`; arbitrary values use rem.
- No inline `style=""` except for a value computed at runtime.

Build: `internal/web/tailwind/build.sh` writes `static/app.css`; `--watch` rebuilds on change.
Look: `deploy/shots/shots.mjs` photographs every page, light and dark, desktop and phone.
