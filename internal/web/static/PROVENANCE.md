# What is vendored here, and why

Served from the binary, never from a content delivery network. An air-gapped
install cannot fetch a CDN, and a page that reaches one is a page whose contents
somebody else can change after review.

| file | what it is | size | sha256 |
| --- | --- | --- | --- |
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
| people/jordan.webp | a portrait for the landing page's example room, cut from an illustration supplied for gusto (2026-11-16), 160px | 8178 | b0afdb37093393452c7425813fbf1cb55a940006d5b6a1160caac530a637f4c1 |
| people/maya.webp | a portrait for the landing page's example room, cut from an illustration supplied for gusto (2026-11-16), 160px | 9370 | 9798bab7070f965f3555af52b1ac343201db6a3f93dc0a3bf64b7aae0599ffa7 |
| people/priya.webp | a portrait for the landing page's example room, cut from an illustration supplied for gusto (2026-11-16), 160px | 8580 | 74ec39a976f394536e6c8ffc3e550b12b7448f7a1a9f399a14d4a6a6495ff632 |
| people/sam.webp | a portrait for the landing page's example room, cut from an illustration supplied for gusto (2026-11-16), 160px | 7442 | 72e86b8e7613eee619b578397001aa60d7f2fa15590f2973dea1b9e81f571b02 |
| landing/hero-dark.webp | the landing page hero's background in the dark theme, from an image supplied for gusto (2026-11-16), 1672px, WebP quality 82 | 9198 | d6cb770e67e58c743eeee1c4dea769287373031c0f81c58b1105745ef37651e3 |
| landing/hero-light.webp | the landing page hero's background in the light theme, from an image supplied for gusto (2026-11-16), 1672px, WebP quality 82 | 7384 | 9a1c9cc54fd9233fbf7a068be82d2bda9055b486a598cc453b3dc9eb2b1f1952 |
| brands/google.svg | Google's G mark for the Continue with Google button on gusto cloud's sign-in and sign-up pages, in the four colours Google's sign-in branding guidelines give it (the mark is Google's trademark) | 697 | 8afaa999d05d43f350116941a02298eb07d4677c86ec76cd7eb1e42d4555c1a6 |
| models/gemini.svg | Google Gemini's mark for the landing page's example models page, from the npm package simple-icons 16.38.0 (CC0 1.0; the mark is its owner's trademark), filled #8E75B2 (fetched 2026-11-16) | 416 | 404eba6940a54e63d40edcce2d2e7cb2b3dbfec765e7a1d523662b6f4e0d6747 |
| models/kimi.svg | Kimi's mark for the landing page's example models page, from the npm package simple-icons 16.38.0 (CC0 1.0; the mark is its owner's trademark), filled #000000 (fetched 2026-11-16) | 669 | e050b0cd0c29cdd91c049d1fe77d0296ee80b316f781d50a0323e2eb1b7c09a1 |
| models/qwen.svg | Qwen's mark for the landing page's example models page, from the npm package simple-icons 16.38.0 (CC0 1.0; the mark is its owner's trademark), filled #6950EF (fetched 2026-11-16) | 820 | 189f77fb476f0a399e22ec968b05f6d24ceb9cad258c5cb3355b114f443757d2 |

Fetched 2026-09-26. Check them:

```bash
shasum -a 256 internal/web/static/*
```

IBM Plex Sans, the text and display face, IBM Plex Serif, the face of the figures that matter, and JetBrains Mono, the code face, are vendored, so a page looks the same on every machine.
