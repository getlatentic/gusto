# Shared Tailwind sources

The React app in `web/` draws every page of this server. Its stylesheet is `web/src/index.css`. The files here are what other
stylesheets share with it.

| File | Job |
| --- | --- |
| `../static/tokens.css` | The `--gu-*` colour, type and spacing tokens, switched by `data-theme` and `data-accent`. |
| `theme.css` | Maps the tokens to Tailwind names (`bg-canvas`, `text-ink`, `border-line`). gusto cloud's stylesheet imports it. |
| `base.css` | Element defaults for the operator page: body, headings, tables, the focus ring. |
| `prose.css` | Rendered Markdown. The React app imports it. |
| `operator.css` | The operator page's stylesheet, built from the classes in `internal/operator/templates`. |

Build: `internal/web/tailwind/build.sh` writes `static/operator.css`. `--watch` rebuilds on change.
