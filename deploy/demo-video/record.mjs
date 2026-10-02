// Records the demo as a person already in the platform sees it, on the live front door, into recording/haas-demo.webm,
// with the waits it had to make marked in recording/segments.json so encode.sh can speed them up. The demo is started
// off camera, as John Smith of Future Acme Bank, and the recording begins inside the organisation. Nothing is faked:
// each step is the page the person gets. End every demo on the host first: one address has one demo, and only a few run.
//
//   PLAYWRIGHT_CORE=<path to playwright-core> CHROME=<a chromium to use, when the library's own is not installed> node record.mjs [base url] [an ended demo's address]
import { createRequire } from "node:module";
import { mkdirSync, rmSync, writeFileSync, readdirSync, renameSync } from "node:fs";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_CORE || "playwright-core");

const base = process.argv[2] || "https://agentcloud.getlatentic.com";
const ended = process.argv[3] || "";
const person = { name: "John Smith", email: "john.smith@futureacmebank.com", company: "Future Acme Bank" };
const task = "Create a loan repayment calculator in a folder called loan-calculator, with an index.html I can open";
const folder = "loan-calculator";
const out = new URL("./recording/", import.meta.url).pathname;
rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });

const browser = await chromium.launch({ headless: true, executablePath: process.env.CHROME || undefined });

// The demo is started off camera: the video is of the platform, where a person already is. Only the sign-in it
// leaves behind is carried over.
const setup = await browser.newContext({ viewport: { width: 1280, height: 720 } });
const first = await setup.newPage();
await first.goto(base + "/demo");
await first.fill('input[name="name"]', person.name);
await first.fill('input[name="email"]', person.email);
await first.fill('input[name="company"]', person.company);
// The bot check answers by itself in a browser, a moment after the page is drawn, and the form is sent with its answer.
if (await first.locator(".cf-turnstile").count()) {
  await first.waitForFunction(() => document.querySelector('[name="cf-turnstile-response"]')?.value, null, { timeout: 30000 });
}
await first.click('button[type="submit"]');
await first.waitForSelector("a.choice", { timeout: 480000 });
const home = first.url();
const signedIn = await setup.storageState();
await setup.close();

const context = await browser.newContext({ viewport: { width: 1280, height: 720 }, storageState: signedIn, recordVideo: { dir: out, size: { width: 1280, height: 720 } } });
const started = Date.now();
const page = await context.newPage();
const segments = [];
const since = () => (Date.now() - started) / 1000;

// A caption over the top of the page, kept across navigations.
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

// 1. In the organisation, choosing an agent.
await page.goto(home);
await say("Choose the agent to work with. Each gets the same workspace and controls");
await pause(5);
await page.click('a.choice[href^="/instances/new"]');
await page.waitForSelector("button.cta");
await say("Start it. The agent gets an isolated workspace of its own");
await pause(4);
await page.click("button.cta");
await page.waitForSelector("textarea");
await say("Only the models your organisation allows, and the model key never enters the sandbox");
await pause(4);
await page.click("textarea");
await page.keyboard.type(task, { delay: 45 });
await pause(1);
await page.keyboard.press("Enter");
await say("The agent works inside its sandbox and writes the files there");
await waiting("the agent works", () => page.waitForSelector("form.preview button", { timeout: 360000 }));
await say("Every step it took is recorded, by the names of its files");
await page.click("details.steps summary").catch(() => {});
await pause(5);

// 2. What it built.
await say("Preview what it built, shared only with your organisation");
await pause(2);
await page.click("form.preview button");
await waiting("starting the preview", () => page.waitForSelector("a:has-text('Open the app')", { timeout: 420000 }));
const app = await page.getAttribute("a:has-text('Open the app')", "href");
await pause(2);
await page.goto(new URL(app, home).href, { waitUntil: "domcontentloaded", timeout: 90000 });
await say("A loan calculator, running in the agent's workspace");
await pause(7);

// 3. The admin side.
await page.goto(new URL("/admin", home).href);
await say("The admin console: what has been spent, who is in, what is running");
await pause(5);
await page.goto(new URL("/admin/models", home).href);
await say("Which models are approved, what each costs, and who may use which");
await pause(6);
await page.goto(new URL("/admin/activity", home).href);
await say("A record of what happened, who did it, and when");
await pause(5);
await page.goto(new URL("/admin/people", home).href);
await say("People and their roles");
await pause(4);

// 4. The end.
if (ended) {
  await page.goto(ended);
  await say("When the demo ends, the organisation and everything in it is removed");
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
