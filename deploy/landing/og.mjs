// Draws the image a link to the landing page shows, 1200 by 630, into internal/cloud/static/og.png from og.html, in the site's own
// font and colors:
//
//   PLAYWRIGHT_CORE=<path to playwright-core> CHROME=<a chromium, when the library's own is not installed> node og.mjs
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_CORE || "playwright-core");
const browser = await chromium.launch({ executablePath: process.env.CHROME || undefined });
const page = await browser.newPage({ viewport: { width: 1200, height: 630 } });
await page.goto(new URL("./og.html", import.meta.url).href);
await page.evaluate(() => document.fonts.ready);
await page.screenshot({ path: fileURLToPath(new URL("../../internal/cloud/static/og.png", import.meta.url)), type: "png" });
await browser.close();
