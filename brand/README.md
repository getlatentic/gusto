# The gusto brand

The mark is `gusto-mark.svg`. Everything else is drawn from it by `build.sh`.

| File | Use |
| --- | --- |
| `gusto-mark.svg` | The mark in Relay Green. The g is a hole, so the page shows through it. |
| `gusto-mark-mono.svg` | The mark in `currentColor`, for a one-color place such as the menu bar. |
| `gusto-lockup.svg` | The mark and the word, for a light page. |
| `gusto-lockup-reversed.svg` | The mark and the word, for a dark page. |

The word is Sora Bold, outlined from `brand/fonts/sora.woff2`.

## Colors

| Name | Hex | Where |
| --- | --- | --- |
| Relay Green | `#00A85A` | The brand color, buttons, the mark |
| Forest | `#00753F` | Green text and pressed states |
| Volt | `#D8FF36` | One highlight on a page. Never text on a light page. |
| Carbon | `#0E1511` | Text on light, the dark page, the label on a green button |
| Snow | `#F5FBF6` | The light page, text on dark |

`internal/web/static/tokens.css` holds the full ramps and the light and dark roles.

## Rebuild the icons

```bash
brand/build.sh
```

It needs `rsvg-convert` and `magick`. On a Mac it also writes the `.icns` file.

The social image is `deploy/landing/og.html`, drawn into `internal/cloud/static/og.png` by `deploy/landing/og.mjs`.
