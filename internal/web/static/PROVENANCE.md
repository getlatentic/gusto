# What is vendored here, and why

Served from the binary, never from a content delivery network. An air-gapped
install cannot fetch a CDN, and a page that reaches one is a page whose contents
somebody else can change after review.

| file | what it is | size | sha256 |
| --- | --- | --- | --- |
| htmx.min.js | htmx 2.0.11, from https://cdn.jsdelivr.net/npm/htmx.org@2/dist/htmx.min.js | 52182 | d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717 |
| tokens.css | gusto's design tokens | 4808 | c8e8196ffaed04d92e0383e7ba61edbf2c9630ce4505a66006a80cf842a0bf7b |
| ibm-plex-sans.woff2 | IBM Plex Sans, weights 100 to 700, latin subset, SIL Open Font License 1.1, from the npm package @fontsource-variable/ibm-plex-sans 5.3.0 (fetched 2026-11-15) | 45712 | e2291e842cf5af167122a22881a740c7f2dda7716f1e8cd76680264f4a859470 |
| ibm-plex-serif-400.woff2 | IBM Plex Serif, weight 400, latin subset, SIL Open Font License 1.1, from the npm package @fontsource/ibm-plex-serif 5.3.0 (fetched 2026-11-15) | 19580 | cb2c5eee2c0a43ff30d2365407c7bc8b20e3bd90720a4a64102ba0b328022a02 |
| ibm-plex-serif-500.woff2 | IBM Plex Serif, weight 500, latin subset, SIL Open Font License 1.1, from the npm package @fontsource/ibm-plex-serif 5.3.0 (fetched 2026-11-15) | 20308 | 677082fc2e9ee60701b874fe5983c98ea9fb3b588cc1b02058ff9bf29ea783e4 |
| ibm-plex-serif-600.woff2 | IBM Plex Serif, weight 600, latin subset, SIL Open Font License 1.1, from the npm package @fontsource/ibm-plex-serif 5.3.0 (fetched 2026-11-15) | 20516 | e279e4f8baa0d4634d98868092f2198ae0dcd9e142c935822c385b9843352a9b |
| jetbrains-mono.woff2 | JetBrains Mono, weights 100 to 800, latin subset, SIL Open Font License 1.1, from the npm package @fontsource-variable/jetbrains-mono 5.3.0 (fetched 2026-11-15) | 40404 | 18be452724bfdc236c074ca94a249a7f41a86752c7d04ab258ce9ed5651f6a7e |
| mark.png | gusto's mark, drawn from brand/gusto-mark.svg at 192px wide | 6622 | 1210d55ec5948a7f7c12a8eda5261e7d669bddc0498ed5829476ccd96d2883fd |
| mark.svg | gusto's mark (brand/gusto-mark.svg) | 1353 | 162a8be4516144137fdfc650ee0b6fa3d4da1cb8b5bbe1b641cb4a8849b00c6b |
| favicon-32.png | the mark at 32px, for the browser tab | 1080 | 2f76291e40d5f50aee45d619636a994e04afbaee508448db1a9fc987815e7fb0 |
| apple-touch-icon.png | the mark on the page color at 180px, for a phone's home screen | 3437 | a9787f059c462ee5f967b8f4a1117cbd90b97031cdcee2c12100ab18b202ebb6 |
| highlight.min.js | highlight.js 11.12.0, the common-languages browser build, BSD 3-Clause, from the npm package @highlightjs/cdn-assets (fetched 2026-10-09) | 129254 | 8ab71eb09c51f501e5e25157d9cff100e46cc29bcbfc744d0b746d451fca7f53 |
| centrifuge.js | centrifuge-js 5.7.4, the browser client of the Centrifuge server library gusto serves rooms with, MIT, Centrifugal Labs LTD, from the npm package centrifuge `dist/centrifuge.js` (fetched 2026-10-26; the original is 77535 bytes with sha256 efbacc3805ecf7cd68cbb75d99e88cc5487c13b4a29330dc671dacea900d9dcd, and this file is it without its last line, a `sourceMappingURL` comment to a map that is not shipped). The server side is `github.com/centrifugal/centrifuge` v0.40.1 in go.mod | 77496 | 6eb431bdaaa4b1f84b2aca9c499cbbfa2d2b3653f4579a86973e1143200965ba |
| centrifuge.LICENSE | the MIT licence of centrifuge-js 5.7.4, as the package ships it | 1087 | 8212f3cd2bcd2cab7e66f5337926e11c3f99822d2016ae41fb1b4e084e5c5128 |

Fetched 2026-09-26. Check them:

```bash
shasum -a 256 internal/web/static/*
```

IBM Plex Sans, the text and display face, IBM Plex Serif, the face of the figures that matter, and JetBrains Mono, the code face, are vendored, so a page looks the same on every machine.

The icons in `templates/layout.html` and `static/explorer.js` are from Lucide (https://lucide.dev, ISC license), on its 24 unit grid.
