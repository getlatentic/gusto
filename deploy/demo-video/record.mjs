// Records the demo as a visitor sees it, on the live front door, into recording/haas-demo.webm, with the waits
// it had to make marked in recording/segments.json so encode.sh can speed them up. Nothing is faked: each step is the
// page the visitor gets. A demo is started with a new address each time, which a person answers to.
//
//   PLAYWRIGHT_CORE=<path to playwright-core> CHROME=<a chromium to use, when the library's own is not installed> node record.mjs [base url] [an ended demo's address]
import { createRequire } from "node:module";
import { mkdirSync, rmSync, writeFileSync, readdirSync, renameSync } from "node:fs";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_CORE || "playwright-core");

const base = process.argv[2] || "https://agentcloud.getlatentic.com";
const ended = process.argv[3] || "";
const address = `tosinamuda+video${Date.now() % 100000}@gmail.com`;
const out = new URL("./recording/", import.meta.url).pathname;
rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });

const browser = await chromium.launch({ headless: true, executablePath: process.env.CHROME || undefined });
const context = await browser.newContext({ viewport: { width: 1280, height: 720 }, recordVideo: { dir: out, size: { width: 1280, height: 720 } } });
const started = Date.now();
const page = await context.newPage();
const segments = [];
const since = () => (Date.now() - started) / 1000;

// A caption over the bottom of the page, kept across navigations.
await page.addInitScript(() => {
  window.__caption = (text) => {
    let bar = document.getElementById("__cap");
    if (!text) { if (bar) bar.remove(); return; }
    if (!bar) {
      bar = document.createElement("div");
      bar.id = "__cap";
      bar.style.cssText = "position:fixed;left:50%;top:74px;transform:translateX(-50%);pointer-events:none;z-index:2147483647;max-width:min(880px,90vw);padding:14px 22px;border-radius:12px;background:rgba(31,26,23,.92);color:#fff;font:600 22px/1.35 system-ui,sans-serif;text-align:center;box-shadow:0 6px 24px rgba(0,0,0,.3)";
      document.body.appendChild(bar);
    }
    bar.textContent = text;
  };
});
const say = async (text) => { await page.evaluate((t) => window.__caption && window.__caption(t), text).catch(() => {}); };
const pause = (seconds) => page.waitForTimeout(seconds * 1000);
// waiting is a stretch of the video that encode.sh plays fast.
async function waiting(label, action) {
  const from = since();
  await action();
  segments.push({ label, from, to: since() });
}
async function typed(selector, text) { await page.click(selector); await page.keyboard.type(text, { delay: 70 }); }

// What the page showed when a step failed is kept beside the video, since a wait that times out says only what it
// waited for.
const failed = async (error) => {
  await page.screenshot({ path: out + "failed.png" }).catch(() => {});
  console.error(error);
  console.error(page.url());
  console.error((await page.innerText("body").catch(() => "")).slice(0, 1200));
  process.exit(1);
};
process.on("unhandledRejection", failed);
process.on("uncaughtException", failed);

// 1. The page.
await page.goto(base + "/");
await say("haas gives each person on a team coding agents that work in a computer of their own");
await pause(5);
await page.evaluate(() => document.getElementById("how").scrollIntoView({ behavior: "smooth" }));
await say("Start a demo: no sign-up to finish");
await pause(4);
await page.evaluate(() => document.getElementById("control").scrollIntoView({ behavior: "smooth" }));
await say("Your models, your limits, a signed record of every job");
await pause(4);
await page.evaluate(() => window.scrollTo({ top: 0, behavior: "smooth" }));
await pause(1.5);
await page.click("a.cta.small, a.cta");

// 2. Start.
await page.waitForSelector('input[name="name"]');
await say("A name, a work email and a company");
await typed('input[name="name"]', "Ada Lovelace");
await typed('input[name="email"]', address);
await typed('input[name="company"]', "Acme Research");
await pause(1.5);
await page.click('button[type="submit"]');
await say("Your time starts when your machine is ready, not before");
await waiting("starting the demo", () => page.waitForURL(/\/$/, { timeout: 420000, waitUntil: "domcontentloaded" }).then(() => page.waitForSelector("a.choice", { timeout: 60000 })));
const home = page.url();
await say("A countdown stays on every page, with one extension");
await pause(5);

// 3. An agent.
await page.click("a.choice.hue-engineering");
await page.waitForSelector("button.cta");
await say("Start an agent. It gets a computer of its own");
await pause(4);
await page.click("button.cta");
await page.waitForSelector("textarea");
await say("Sealed, and the model key stays with haas");
await pause(3);
await page.click("textarea");
await page.keyboard.type("create a 2048 app", { delay: 90 });
await pause(1);
await page.keyboard.press("Enter");
await say("It works in the sandbox and writes the files there");
await waiting("the agent works", () => page.waitForSelector("form.preview button", { timeout: 360000 }));
await say("Every step it took is shown, by the names of its files");
await page.click("details.steps summary").catch(() => {});
await pause(5);

// 4. What it built.
await say("Preview what it built");
await pause(2);
await page.click("form.preview button");
await waiting("starting the preview", () => page.waitForSelector("a:has-text('Open the app')", { timeout: 420000 }));
const app = await page.getAttribute("a:has-text('Open the app')", "href");
await say("Shared only with people in your organisation");
await pause(3);
await page.goto(new URL(app, home).href);
await pause(2);
await say("Your 2048, running in the agent's computer");
for (const key of ["ArrowLeft", "ArrowUp", "ArrowRight", "ArrowDown", "ArrowLeft", "ArrowUp"]) { await page.keyboard.press(key); await pause(0.6); }
await pause(3);

// 5. The admin side.
await page.goto(new URL("/admin", home).href);
await say("The admin console: what has been spent, who is in, what is running");
await pause(5);
await page.goto(new URL("/admin/models", home).href);
await say("Which models are allowed, what each costs, and who may use which");
await pause(6);
await page.goto(new URL("/admin/people", home).href);
await pause(3);

// 6. The end.
if (ended) {
  await page.goto(ended);
  await say("When the time is up, the organisation and everything in it is deleted");
  await pause(6);
}
await say("");
await pause(1);

await context.close();
await browser.close();
const file = readdirSync(out).find((name) => name.endsWith(".webm"));
renameSync(out + file, out + "haas-demo.webm");
writeFileSync(out + "segments.json", JSON.stringify(segments, null, 1));
console.log(`recorded ${since().toFixed(0)} s, ${segments.length} waits: ${segments.map((s) => `${s.label} ${(s.to - s.from).toFixed(0)}s`).join(", ")}`);
