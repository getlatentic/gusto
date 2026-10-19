// Records the demo as a person already in the platform sees it, on the live front door, into recording/gusto-demo.webm,
// with the waits it had to make marked in recording/segments.json so encode.sh can speed them up. The demo is started
// off camera, as John Smith of Future Acme Bank, and the recording begins inside the organisation. Nothing is faked:
// each step is the page the person gets. End every demo on the host first: one address has one demo, and only a few run.
//
// Each caption is also written, with the time it was said, to recording/marks.json, so that edit/ can draw its own captions and
// aim its camera. CAPTIONS=off records the page without the captions drawn on it. HEADED=1 shows the browser, which the bot check
// is more likely to pass.
//
//   PLAYWRIGHT_CORE=<path to playwright-core> CHROME=<a chromium to use, when the library's own is not installed> node record.mjs [base url] [an ended demo's address]
import { createRequire } from "node:module";
import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.PLAYWRIGHT_CORE || "playwright-core");

const base = process.argv[2] || "https://gusto.getlatentic.com";
const ended = process.argv[3] || "";
const person = { name: "John Smith", email: "john.smith@futureacmebank.com", company: "Future Acme Bank" };
const task = "Create a loan repayment calculator as a single file, index.html, in the current folder. It has fields for the loan amount, the yearly interest rate and the years, a Calculate button, and shows the monthly payment.";
const folder = "loan-calculator";
const out = new URL("./recording/", import.meta.url).pathname;
rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });

const pixels = Number(process.env.PIXELS || 3);
const captions = process.env.CAPTIONS !== "off";
const browser = await chromium.launch({ headless: !process.env.HEADED, executablePath: process.env.CHROME || undefined, args: [`--force-device-scale-factor=${pixels}`] });

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
await first.waitForSelector("a.choice", { timeout: 480000 }).catch(async (error) => {
  console.error(error.message);
  console.error(first.url());
  console.error((await first.innerText("body").catch(() => "")).slice(0, 800));
  process.exit(1);
});
const home = first.url();
const signedIn = await setup.storageState();
await setup.close();

const context = await browser.newContext({ viewport: { width: 1280, height: 720 }, deviceScaleFactor: pixels, storageState: signedIn });
let started = Date.now();
const page = await context.newPage();
const segments = [];
const since = () => (Date.now() - started) / 1000;

// The browser's own screencast, frame by frame, at the pixels the screen has: the browser's recorder keeps the page's size and
// compresses it hard. Each frame says when it was drawn, and the video is made from them when the recording ends.
mkdirSync(out + "frames");
const frames = [];
const screencast = await context.newCDPSession(page);
screencast.on("Page.screencastFrame", async (frame) => {
  if (!frames.length) started = Date.now();
  const name = `frames/${String(frames.length).padStart(6, "0")}.jpg`;
  writeFileSync(out + name, Buffer.from(frame.data, "base64"));
  frames.push({ name, at: frame.metadata.timestamp });
  await screencast.send("Page.screencastFrameAck", { sessionId: frame.sessionId }).catch(() => {});
});
await screencast.send("Page.startScreencast", { format: "jpeg", quality: 92, maxWidth: 1280 * pixels, maxHeight: 720 * pixels, everyNthFrame: 1 });

// A caption over the top of the page, and the pointer, kept across navigations. The browser's screencast draws no pointer, so the
// page draws one that follows the mouse, with a ring where it clicks.
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
  const pointer = () => {
    let el = document.getElementById("__ptr");
    if (!el && document.documentElement) {
      el = document.createElement("div");
      el.id = "__ptr";
      el.style.cssText = "position:fixed;left:0;top:0;width:30px;height:30px;z-index:2147483646;pointer-events:none;transform:translate(-200px,-200px);filter:drop-shadow(0 2px 3px rgba(0,0,0,.35))";
      el.innerHTML = '<svg viewBox="0 0 24 24" width="30" height="30"><path d="M4 2l0 17 4.5-4.2 3 6.7 3.1-1.4-3-6.6 6.1-.3z" fill="#171717" stroke="#fff" stroke-width="1.5" stroke-linejoin="round"/></svg>';
      document.documentElement.appendChild(el);
    }
    return el;
  };
  addEventListener("mousemove", (e) => { const el = pointer(); if (el) el.style.transform = `translate(${e.clientX - 4}px, ${e.clientY - 2}px)`; }, true);
  addEventListener("mousedown", (e) => {
    const ring = document.createElement("div");
    ring.style.cssText = `position:fixed;left:${e.clientX - 22}px;top:${e.clientY - 22}px;width:44px;height:44px;border-radius:50%;border:3px solid #FF8A1F;z-index:2147483645;pointer-events:none`;
    document.documentElement.appendChild(ring);
    ring.animate([{ transform: "scale(.4)", opacity: 1 }, { transform: "scale(1.5)", opacity: 0 }], { duration: 520, easing: "ease-out" }).onfinish = () => ring.remove();
  }, true);
});

// Where a step is on the page, as fractions of the page's size, so that edit/ can aim its camera at it. A target is a selector, or
// { all: selector } for every element it matches.
const rectOf = async (targets, pad = 0) => {
  const boxes = [];
  for (const target of targets || []) {
    const locator = page.locator(typeof target === "string" ? target : target.all);
    const found = typeof target === "string" ? [locator.first()] : await locator.all();
    for (const each of found) {
      const box = await each.boundingBox({ timeout: 1500 }).catch(() => null);
      if (box) boxes.push(box);
    }
  }
  if (!boxes.length) return undefined;
  const left = Math.max(0, Math.min(...boxes.map((b) => b.x)) - pad);
  const top = Math.max(0, Math.min(...boxes.map((b) => b.y)) - pad);
  const right = Math.min(1280, Math.max(...boxes.map((b) => b.x + b.width)) + pad);
  const bottom = Math.min(720, Math.max(...boxes.map((b) => b.y + b.height)) + pad);
  if (right <= left || bottom <= top) return undefined;
  return { x: left / 1280, y: top / 720, w: (right - left) / 1280, h: (bottom - top) / 720 };
};
// A take ends when the click or the navigation that leaves its page happens, and the next begins when its page is drawn: the footage
// between, a page loading, is left out of the edit. leaving is when the last such step happened.
const marks = [];
let leaving;
const say = async (text, targets, pad) => {
  const rect = await rectOf(targets, pad);
  marks.push({ t: since(), text, ...(rect ? { rect } : {}), ...(leaving !== undefined ? { cut: leaving } : {}) });
  leaving = undefined;
  if (captions) await page.evaluate((t) => window.__caption && window.__caption(t), text).catch(() => {});
};
const pause = (seconds) => page.waitForTimeout(seconds * 1000);

// The pointer moves to an element over most of a second, rests on it, then presses. A click that opens another page ends the take.
let pointer = { x: 640, y: 360 };
page.on("domcontentloaded", () => { page.mouse.move(pointer.x, pointer.y).catch(() => {}); });
async function clickOn(selector, { rest = 0.45, leaves = true } = {}) {
  const target = page.locator(selector).first();
  await target.scrollIntoViewIfNeeded().catch(() => {});
  const box = await target.boundingBox();
  const to = { x: box.x + box.width * 0.5, y: box.y + box.height * 0.55 };
  const from = pointer;
  const steps = 32;
  for (let i = 1; i <= steps; i++) {
    const k = i / steps;
    const e = k < 0.5 ? 2 * k * k : 1 - Math.pow(-2 * k + 2, 2) / 2;
    pointer = { x: from.x + (to.x - from.x) * e, y: from.y + (to.y - from.y) * e };
    await page.mouse.move(pointer.x, pointer.y);
    await page.waitForTimeout(24);
  }
  await pause(rest);
  await page.mouse.down();
  await page.waitForTimeout(110);
  await page.mouse.up();
  // The page that a click leaves is gone a few frames after the button comes up, so the take ends there.
  if (leaves) leaving = since() - 0.06;
  await page.waitForTimeout(260);
}
const go = async (address, options) => {
  leaving = since();
  await page.goto(address, options);
};
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
await say("Choose the agent to work with. Each gets the same workspace and controls", [{ all: 'a.choice[href^="/instances/new"]' }]);
await pause(3.2);
await clickOn('a.choice[href="/instances/new?harness=claude"]');
await page.waitForSelector("button.cta");
await say("Start it. The agent gets an isolated workspace of its own", ["form.start-form"]);
await pause(2.4);
await clickOn("button.cta");
await page.waitForSelector("textarea");
await say("Only the models your organisation allows, and the model key never enters the sandbox", ["textarea"]);
await pause(1.5);
await clickOn("textarea", { rest: 0.2, leaves: false });
await page.keyboard.type(task, { delay: 45 });
await pause(0.8);
await page.keyboard.press("Enter");
leaving = since() - 0.05;
await say("The agent works inside its sandbox and writes the files there", ["text=Key stays with"], 14);
await waiting("the agent works", () => page.waitForSelector("form.preview button", { timeout: 360000 }));
await say("Every step it took is recorded, by the names of its files", ["details.steps"]);
await pause(1.2);
await clickOn("details.steps summary", { leaves: false }).catch(() => {});
await pause(3);

// 2. What it built.
await say("Preview what it built, shared only with your organisation", ["form.preview button"], 24);
await pause(1.4);
await clickOn("form.preview button");
await waiting("starting the preview", () => page.waitForSelector("a:has-text('Open the app')", { timeout: 420000 }));
const app = await page.getAttribute("a:has-text('Open the app')", "href");
await pause(1);
await go(new URL(app, home).href, { waitUntil: "domcontentloaded", timeout: 90000 });
await say("A loan calculator, running in the agent's workspace");
await pause(1.4);
// The calculator is what the model wrote, so what is done to it is only what it has: three fields and a button. A field may come filled in,
// so what is in it is selected before the value is typed.
const fields = page.locator("input");
if ((await fields.count()) >= 3 && (await page.locator("button").count()) >= 1) {
  for (const [i, value] of ["250000", "6.5", "30"].entries()) {
    await clickOn(`input >> nth=${i}`, { rest: 0.25, leaves: false });
    await page.keyboard.press("ControlOrMeta+a");
    await page.keyboard.type(value, { delay: 90 });
  }
  await clickOn("button >> nth=-1", { leaves: false });
}
await pause(2.4);

// 3. The admin side.
await go(new URL("/admin", home).href);
await say("The admin console: what has been spent, who is in, what is running", ["text=/spent in 20/"], 70);
await pause(3.4);
await go(new URL("/admin/models", home).href);
await say("Which models are approved, what each costs, and who may use which", ["table"], 20);
await pause(3.4);
await go(new URL("/admin/activity", home).href);
await say("A record of what happened, who did it, and when", ["table"], 20);
await pause(3.4);
await go(new URL("/admin/people", home).href);
await say("People and their roles", ["table"], 30);
await pause(3);

// 4. The end.
if (ended) {
  await go(ended);
  await say("When the demo ends, the organisation and everything in it is removed");
  await pause(5);
}
await say("");
await pause(1);

await screencast.send("Page.stopScreencast").catch(() => {});
await context.close();
await browser.close();
makeVideo(frames);
writeFileSync(out + "segments.json", JSON.stringify(segments, null, 1));
writeFileSync(out + "marks.json", JSON.stringify(marks, null, 1));
console.log(`recorded ${since().toFixed(0)} s, ${segments.length} waits: ${segments.map((s) => `${s.label} ${(s.to - s.from).toFixed(0)}s`).join(", ")}`);

// raw.mp4 shows each frame for as long as it was on the screen, at 30 frames a second.
function makeVideo(frames) {
  const list = frames.map((frame, i) => `file '${frame.name}'\nduration ${((frames[i + 1]?.at ?? frame.at + 0.1) - frame.at).toFixed(4)}`).join("\n");
  writeFileSync(out + "frames.txt", list + `\nfile '${frames[frames.length - 1].name}'\n`);
  const made = spawnSync("ffmpeg", ["-y", "-loglevel", "error", "-f", "concat", "-safe", "0", "-i", out + "frames.txt", "-vf", "fps=30", "-c:v", "libx264", "-crf", "14", "-preset", "veryfast", "-pix_fmt", "yuv420p", out + "raw.mp4"], { stdio: "inherit" });
  if (made.status !== 0) throw new Error("ffmpeg could not make the video from the frames");
  rmSync(out + "frames", { recursive: true, force: true });
  rmSync(out + "frames.txt");
}
