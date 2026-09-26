# What is vendored here, and why

Served from the binary, never from a content delivery network. An air-gapped
install cannot fetch a CDN, and a page that reaches one is a page whose contents
somebody else can change after review.

| file | htmx.min.js |
| --- | --- |
| version | 2.0.11 |
| source | https://cdn.jsdelivr.net/npm/htmx.org@2/dist/htmx.min.js |
| size | 52182 bytes |
| sha256 | d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717 |
| fetched | 2026-09-26 |

Check it:

```bash
shasum -a 256 internal/web/static/htmx.min.js
```
