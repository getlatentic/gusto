# Styling the app

All styling is Tailwind v4. There is no other stylesheet for the app besides the runtime tokens.

| File | Job |
| --- | --- |
| `static/tokens.css` | The `--gu-*` colour, type and spacing tokens, switched by `data-theme` and `data-accent`. |
| `theme.css` | Maps the tokens to Tailwind names (`bg-canvas`, `text-ink`, `border-line`, `rounded-md`, `shadow-1`). |
| `base.css` | Element defaults: body, headings, tables, focus ring, `svg.i`. |
| `prose.css` | Rendered Markdown. |
| `components.css` | Every shared component as a Tailwind component class in `@layer components`: `btn`, `input`, `select`, `card`, `badge`, `dialog`, `dropdown`, `data-table`, `tab`, `nav-item` and the rest, measured against Kylon. |
| `../templates/ui.html` | The shared pieces of markup (`ui-flash`, `ui-bar-open`, `ui-dialog-head`, `ui-tabs`, `ui-state`, `ui-chev`), which use those classes. |

Rules:

- A control is its component class: `class="btn"`, `class="card"`, `class="dropdown-item"`. Utilities in the template are for the page's own layout, and may adjust one property of a component, since utilities outrank `@layer components`.
- A new kind of control is a new class in `components.css` and an example on the kit page (`/admin/ui-kit`). `uikit_test.go` fails a class the kit does not show, and `kitguard_test.go` fails a template or script that draws a dialog, menu, table, field, card, badge, tab or solid button in utilities.
- Use token-backed names only (`bg-surface`, `text-muted`, `border-line`). No hex, no arbitrary colours.
- Layout adapts to the page width with container variants: `@max-[56rem]:` (phone and narrow panes), never `max-*:` viewport variants.
- A disclosure is `<details class="group">` with a `summary`-class summary and a `ui-chev` chevron. Never style the default marker.
- Class names and data attributes the scripts use stay in the markup: `.scrim`, `#side`, `[data-side-toggle]`, `[data-panel-toggle]`, `[data-look]`, `.picker.open`, and so on. A script that builds markup builds it with the same utilities.
- Sizes are rem. The build rewrites `px` to `rem`; arbitrary values use rem.
- No inline `style=""` except for a value computed at runtime.

Build: `internal/web/tailwind/build.sh` writes `static/app.css`; `--watch` rebuilds on change.
Look: `deploy/shots/shots.mjs` photographs every page, light and dark, desktop and phone.
