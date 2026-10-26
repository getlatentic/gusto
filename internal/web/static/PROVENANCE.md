# What is vendored here, and why

Served from the binary, never from a content delivery network. An air-gapped
install cannot fetch a CDN, and a page that reaches one is a page whose contents
somebody else can change after review.

| file | what it is | size | sha256 |
| --- | --- | --- | --- |
| htmx.min.js | htmx 2.0.11, from https://cdn.jsdelivr.net/npm/htmx.org@2/dist/htmx.min.js | 52182 | d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717 |
| tokens.css | gusto's design tokens | 4236 | ebbb19d06726a10fe86050880f75258b606fc41a8b4780e8d6469dc8e597e399 |
| inter.woff2 | Inter, weights 100 to 900, latin subset, SIL Open Font License 1.1, from the npm package @fontsource-variable/inter 5.3.0 (fetched 2026-10-23) | 48256 | 3100e775e8616cd2611beecfa23a4263d7037586789b43f035236a2e6fbd4c62 |
| noto-sans-mono.woff2 | Noto Sans Mono, weights 100 to 900, latin subset, SIL Open Font License 1.1, from the npm package @fontsource-variable/noto-sans-mono 5.3.0 (fetched 2026-10-19) | 32108 | 71f6c22d5dd256eaa9e48347bda6ac2c65f58980dd372ed5780fbdaca1186331 |
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

Inter, the text and display face, and Noto Sans Mono, the code face, are vendored, so a page looks the same on every machine.

The icons in `templates/layout.html` and `static/explorer.js` are from Lucide (https://lucide.dev, ISC license), on its 24 unit grid.
