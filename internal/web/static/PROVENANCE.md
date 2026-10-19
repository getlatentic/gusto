# What is vendored here, and why

Served from the binary, never from a content delivery network. An air-gapped
install cannot fetch a CDN, and a page that reaches one is a page whose contents
somebody else can change after review.

| file | what it is | size | sha256 |
| --- | --- | --- | --- |
| htmx.min.js | htmx 2.0.11, from https://cdn.jsdelivr.net/npm/htmx.org@2/dist/htmx.min.js | 52182 | d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717 |
| tokens.css | gusto's design tokens | 4236 | ebbb19d06726a10fe86050880f75258b606fc41a8b4780e8d6469dc8e597e399 |
| sora.woff2 | Sora, weights 400 to 700, latin subset, SIL Open Font License 1.1, from https://fonts.gstatic.com/s/sora/v17/xMQ9uFFYT72X5wkB_18qmnndmSdSnh2BAfO5mnuyOo1lfiQwV6-xo6eeIw.woff2 (fetched 2026-09-27) | 25240 | 3902474d3ece89c8580d5ef3325cf58753e6e45a86f9957b4dacc4fce2e7fde1 |
| mark.png | gusto's mark, drawn from brand/gusto-mark.svg at 192px wide | 6622 | 1210d55ec5948a7f7c12a8eda5261e7d669bddc0498ed5829476ccd96d2883fd |
| mark.svg | gusto's mark (brand/gusto-mark.svg) | 1353 | 162a8be4516144137fdfc650ee0b6fa3d4da1cb8b5bbe1b641cb4a8849b00c6b |
| favicon-32.png | the mark at 32px, for the browser tab | 1080 | 2f76291e40d5f50aee45d619636a994e04afbaee508448db1a9fc987815e7fb0 |
| apple-touch-icon.png | the mark on the page colour at 180px, for a phone's home screen | 3437 | a9787f059c462ee5f967b8f4a1117cbd90b97031cdcee2c12100ab18b202ebb6 |
| highlight.min.js | highlight.js 11.12.0, the common-languages browser build, BSD 3-Clause, from the npm package @highlightjs/cdn-assets (fetched 2026-10-09) | 129254 | 8ab71eb09c51f501e5e25157d9cff100e46cc29bcbfc744d0b746d451fca7f53 |

Fetched 2026-09-26. Check them:

```bash
shasum -a 256 internal/web/static/*
```

Sora, the display face, is vendored for headings. Noto Sans and Noto Sans Mono
are **not**. Each stack falls back to the system's own, which is what the tokens
declare. Vendoring them costs about a hundred kilobytes a weight, and nothing
yet needs the exact letterforms.

The icons in `templates/layout.html` and `static/explorer.js` are from Lucide (https://lucide.dev, ISC licence), on its 24 unit grid.
