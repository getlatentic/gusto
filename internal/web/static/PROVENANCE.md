# What is vendored here, and why

Served from the binary, never from a content delivery network. An air-gapped
install cannot fetch a CDN, and a page that reaches one is a page whose contents
somebody else can change after review.

| file | what it is | size | sha256 |
| --- | --- | --- | --- |
| htmx.min.js | htmx 2.0.11, from https://cdn.jsdelivr.net/npm/htmx.org@2/dist/htmx.min.js | 52182 | d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717 |
| tokens.css | Yarngo design tokens v1.0 | 5598 | 465f1ae6476797f18246aae6ad575339e9d9afc6add3548660b54dadc8aeef54 |
| sora.woff2 | Sora, weights 400 to 700, latin subset, SIL Open Font License 1.1, from https://fonts.gstatic.com/s/sora/v17/xMQ9uFFYT72X5wkB_18qmnndmSdSnh2BAfO5mnuyOo1lfiQwV6-xo6eeIw.woff2 (fetched 2026-09-27) | 25240 | 3902474d3ece89c8580d5ef3325cf58753e6e45a86f9957b4dacc4fce2e7fde1 |
| mark.png | the Yarngo mark, resized to the 64px the page renders | 4993 | a5e85a9b0dceacca8e29f02d4aa21f29b5ae5cfc67a6c78d3c0f5edb180fd9cf |

Fetched 2026-09-26. Check them:

```bash
shasum -a 256 internal/web/static/*
```

Sora, the display face, is vendored for headings. Noto Sans and Noto Sans Mono
are **not**. Each stack falls back to the system's own, which is what the tokens
declare. Vendoring them costs about a hundred kilobytes a weight, and nothing
yet needs the exact letterforms.
