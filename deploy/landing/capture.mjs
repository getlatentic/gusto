// Retakes the landing page's screens from the current app at high pixel density, in light and dark: the hero chat and the four
// crops below it (agents, activity, limits, record).
//
//   cd <a folder with playwright installed> && node /path/to/deploy/landing/capture.mjs <out dir>
//
// It starts the server the way deploy/shots/shots.mjs does, with a model that replays the words of a real demo run, has
// the finance conversation, and photographs the pages. Each <name>-<theme>.png in <out dir> is then encoded with cwebp into
// internal/cloud/static/ (hero) or internal/cloud/static/shots/ (the crops).
import { spawn, execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync, rmSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { chromium } from "playwright";

const root = resolve(new URL("../..", import.meta.url).pathname);
const out = resolve(process.argv[2] ?? "capture-out");
const port = 8720, base = `http://127.0.0.1:${port}`, password = "capture";
const work = mkdtempSync(join(tmpdir(), "gusto-capture-"));
const children = [];
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// each server keeps its own change log and other state under the folder it starts in
const start = (cmd, args, env = {}, cwd = work) => { const c = spawn(cmd, args, { cwd, env: { ...process.env, ...env }, stdio: "ignore" }); children.push(c); return c; };
const finish = (code) => { children.forEach((c) => c.kill("SIGTERM")); rmSync(work, { recursive: true, force: true }); process.exit(code); };

const first = { say: "Tell me here, in plain sentences, the three main rules in policy.md.",
  text: "Everyone gets a fixed monthly budget, anything over it needs a manager to approve, and every purchase is listed in the monthly review.", in: 9000, out: 40, wait: 7 };
const turns = [
  { say: "I run finance at a 40-person company. What is the first thing to decide before we give everyone an AI agent? Answer in two sentences.",
    text: "Define your data governance and security policies to ensure proprietary financial information is not leaked to external models. Next, establish clear use cases and success metrics to ensure the investment delivers measurable productivity gains.", in: 7000, out: 55, wait: 7 },
  { say: "Good. Draft a spending policy with a monthly limit per person, save it as policy.md, and tell me the limit you picked in one sentence.",
    text: "The monthly limit is $50 per person.", in: 14000, out: 12, wait: 8 },
  { say: "Now write a two-sentence note I can send the team about it.",
    text: "We are introducing a $50 monthly budget per person for AI tools to support your productivity. Please review the attached `policy.md` for details on usage guidelines and approval processes.", in: 7000, out: 42, wait: 7 },
];

const bin = join(work, "gusto");
execFileSync("go", ["build", "-o", bin, "./cmd/gusto"], { cwd: root, env: { ...process.env, CC: process.env.CC ?? "/usr/bin/clang" }, stdio: "inherit" });
for (const d of ["creds", "agents", "keys"]) mkdirSync(join(work, d));
writeFileSync(join(work, "replies.json"), JSON.stringify([first, ...turns]));
writeFileSync(join(work, "quota.json"), JSON.stringify({ jobs: 30, concurrent: 2 }));
mkdirSync(join(work, "registry"));
writeFileSync(join(work, "creds", "gemma.json"), JSON.stringify({ address: "http://127.0.0.1:9100/v1", auth: "bearer", key: "x", wires: ["chat"] }));
writeFileSync(join(work, "spend.json"), JSON.stringify({ prices: { "google.gemma-4-26b-a4b": { inputPerMillion: 0.13, outputPerMillion: 0.4 } }, teams: {} }));
const ask = "fetch(process.env.GUSTO_MODEL_URL+'/chat/completions',{method:'POST',headers:{'content-type':'application/json',authorization:'Bearer '+process.env.GUSTO_MODEL_TOKEN},body:JSON.stringify({model:'google.gemma-4-26b-a4b',messages:[{role:'user',content:process.env.GUSTO_BRIEF}]})}).then(r=>r.json()).then(j=>console.log(j.choices[0].message.content))";
writeFileSync(join(work, "agents", "opencode.json"), JSON.stringify({
  name: "opencode", title: "OpenCode", what: "Open source, and points at whichever model the deployment gives it.", program: "sh",
  argv: ["-c", `node -e "${ask}" | tee out.txt; grep -q 'monthly limit' out.txt && printf '# Spending policy\\n\\n## Budget\\n\\n- $50 a month per person\\n- Unused budget does not carry over\\n\\n## Approval\\n\\n- Spend over the budget needs a manager\\n- Managers answer within two working days\\n\\n## Review\\n\\n- Every purchase is listed monthly\\n- Finance reviews the list each quarter\\n' > policy.md; rm -f out.txt`],
  env: { GUSTO_MODEL_URL: "{modelUrl}", GUSTO_MODEL_TOKEN: "{modelToken}", GUSTO_BRIEF: "{brief}" }, needs: "key", stream: "plain",
}));
execFileSync(bin, ["keygen", "capture", "-dir", join(work, "keys")], { stdio: "ignore" });
start("python3", [join(root, "deploy/landing/replay.py"), join(work, "replies.json")]);
start(bin, ["serve"], {
  GUSTO_PASSWORD: password, GUSTO_ADDR: `127.0.0.1:${port}`, GUSTO_JOBS: join(work, "jobs"), GUSTO_SESSIONS: join(work, "sessions"),
  GUSTO_SANDBOX: "here", GUSTO_CREDENTIALS: join(work, "creds"), GUSTO_MODELS: "gemma/google.gemma-4-26b-a4b", GUSTO_HARNESSES: join(work, "agents"),
  GUSTO_SIGNING_KEY: join(work, "keys", "capture.key"), GUSTO_KEYRING: join(work, "keys"), GUSTO_SPEND_POLICY: join(work, "spend.json"),
  GUSTO_PROXY_ADDR: "127.0.0.1:8721", GUSTO_QUOTA: join(work, "quota.json"), HOME: work,
});
for (let i = 0; ; i++) {
  try { if ((await fetch(`${base}/healthz`)).ok) break; } catch {}
  if (i > 60) { console.error("the server did not start"); finish(1); }
  await sleep(500);
}

// a second server with nothing on it shows what a person sees first: the agents to start with
const empty = join(work, "empty"), shims = join(work, "shims");
mkdirSync(empty);
mkdirSync(shims);
// the agents that are installed are the ones offered, and a person sees all six on a hosted server
for (const [program, version] of [["claude-agent-acp", "0.94.0"], ["codex-acp", "2.2.2"]]) { writeFileSync(join(shims, program), `#!/bin/sh\necho ${version}\n`, { mode: 0o755 }); }
start(bin, ["serve"], {
  GUSTO_PASSWORD: password, GUSTO_ADDR: "127.0.0.1:8722", GUSTO_JOBS: join(empty, "jobs"), GUSTO_SESSIONS: join(empty, "sessions"),
  GUSTO_SANDBOX: "here", GUSTO_CREDENTIALS: join(work, "creds"), GUSTO_MODELS: "gemma/google.gemma-4-26b-a4b", GUSTO_HARNESSES: join(root, "pkg/harness/adapters"),
  GUSTO_PROXY_ADDR: "127.0.0.1:8723", HOME: empty, PATH: `${shims}:${process.env.PATH}`,
}, empty);

// and a third whose console lists the models an operator has connected, which a server without a registry does not offer
const models = join(work, "models");
mkdirSync(models);
mkdirSync(join(models, "creds"));
// an operator's provider, at an address that is only a name here
writeFileSync(join(models, "creds", "gemma.json"), JSON.stringify({ address: "https://models.example.com/v1", auth: "bearer", key: "x", wires: ["chat"] }));
start(bin, ["serve"], {
  GUSTO_PASSWORD: password, GUSTO_ADDR: "127.0.0.1:8724", GUSTO_JOBS: join(models, "jobs"), GUSTO_SESSIONS: join(models, "sessions"),
  GUSTO_SANDBOX: "here", GUSTO_CREDENTIALS: join(models, "creds"), GUSTO_MODELS: "gemma/google.gemma-4-26b-a4b,gemma/google.gemma-4-31b", GUSTO_REGISTRY: join(work, "registry"),
  GUSTO_HARNESSES: join(root, "pkg/harness/adapters"), GUSTO_SPEND_POLICY: join(work, "spend.json"), GUSTO_PROXY_ADDR: "127.0.0.1:8725", HOME: models,
}, models);

const cookies = async () => {
  const jar = {};
  return { jar, call: async (host, method, path, form) => {
    const res = await fetch(host + path, { method, redirect: "manual",
      headers: { cookie: Object.entries(jar).map(([k, v]) => `${k}=${v}`).join("; "), ...(form ? { "content-type": "application/x-www-form-urlencoded" } : {}) },
      body: form ? new URLSearchParams(form).toString() : undefined });
    for (const line of res.headers.getSetCookie?.() ?? []) { const [pair] = line.split(";"); const [k, v] = pair.split("="); jar[k] = v; }
    return res;
  } };
};
const { jar, call: ask1 } = await cookies();
const call = (method, path, form) => ask1(base, method, path, form);
await call("POST", "/login", { password });
const model = "gemma/google.gemma-4-26b-a4b";
const say = async (thread, words) => {
  await call("POST", `${thread}/say`, { said: words });
  for (let i = 0; i < 60; i++) { await sleep(1000); const page = await (await call("GET", thread)).text(); if (!/Working|Waiting/.test(page) && page.includes(words.slice(0, 20))) { await sleep(1500); break; } }
};
const earlier = (await call("POST", "/instances", { harness: "opencode", model })).headers.get("location");
await say(earlier, first.say);
const thread = (await call("POST", "/instances", { harness: "opencode", model })).headers.get("location");
for (const turn of turns) { await say(thread, turn.say); }
// the thread page no longer links its tasks, so they are read from the job list
const ids = [...new Set(((await (await call("GET", "/jobs")).text()).match(/href="\/jobs\/([^"#?]+)"/g) ?? []).map((m) => m.slice(12, -1)))];
let policyJob = ids[0];
for (const id of ids) { if ((await (await call("GET", `/jobs/${id}`)).text()).includes("policy.md")) { policyJob = id; break; } }

const { jar: freshJar, call: ask2 } = await cookies();
await ask2("http://127.0.0.1:8722", "POST", "/login", { password });
const { jar: modelsJar, call: ask3 } = await cookies();
await ask3("http://127.0.0.1:8724", "POST", "/login", { password });

rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });
const browser = await chromium.launch();

// a crop is a region of the page, at the page's own size: the element's box, or the top of it when it is longer than `tall`
const crop = async (page, name, theme, box, tall = Infinity) => {
  const scroll = await page.evaluate(() => window.scrollY);
  await page.screenshot({ path: join(out, `${name}-${theme}.png`), fullPage: true,
    clip: { x: box.x, y: box.y + scroll, width: box.width, height: Math.min(box.height, tall) } });
};
const card = (locator) => locator.locator("xpath=ancestor::*[contains(@class,'rounded-md')][1]");

for (const theme of ["light", "dark"]) {
  const open = async (viewport, scale, jarOf, host) => {
    const context = await browser.newContext({ viewport, deviceScaleFactor: scale, colorScheme: theme });
    await context.addCookies(Object.entries(jarOf).map(([name, value]) => ({ name, value, url: host })));
    await context.addInitScript((t) => { try { localStorage.setItem("gusto-theme", t); } catch {} }, theme);
    return { context, page: await context.newPage() };
  };

  // the hero: the finance conversation at twice the density
  let { context, page } = await open({ width: 1000, height: 1000 }, 2, jar, base);
  await page.goto(base + thread, { waitUntil: "networkidle" });
  await page.evaluate(() => { const org = document.querySelector("#sidepanel span"); if (org) org.textContent = "demo-relay-labs-89ec"; });
  await page.evaluate(() => { const said = document.getElementById("said"); if (said) said.scrollTop = 0; });
  await page.waitForTimeout(400);
  await page.screenshot({ path: join(out, `hero-${theme}.png`), clip: { x: 0, y: 0, width: 1000, height: 680 } });
  await context.close();

  // the crops are at 2.5x, so each comes out at more than twice the width it is shown at; sizes are the elements' own
  ({ context, page } = await open({ width: 1000, height: 1000 }, 2.5, jar, base));
  await page.goto(base + "/jobs", { waitUntil: "networkidle" });
  let box = await card(page.getByText("Today", { exact: true }).first()).boundingBox();
  await crop(page, "activity", theme, box, 420);
  // the overview puts its cards in two columns on a wide page, which makes the limits card narrow: on a page that is one column it is as wide as it is shown
  await context.close();
  ({ context, page } = await open({ width: 880, height: 1000 }, 2.5, jar, base));
  await page.goto(base + "/admin", { waitUntil: "networkidle" });
  box = await card(page.getByRole("heading", { name: "Limits" })).boundingBox();
  await crop(page, "limits", theme, box);
  await context.close();
  ({ context, page } = await open({ width: 1000, height: 1000 }, 2.5, jar, base));
  await page.goto(base + `/jobs/${policyJob}`, { waitUntil: "networkidle" });
  box = await page.locator("section[aria-label=Summary]").boundingBox();
  await crop(page, "record", theme, box);
  await context.close();

  ({ context, page } = await open({ width: 1000, height: 1000 }, 2.5, modelsJar, "http://127.0.0.1:8724"));
  await page.goto("http://127.0.0.1:8724/admin/models", { waitUntil: "networkidle" });
  box = await card(page.getByText("set by the operator").first()).boundingBox();
  await crop(page, "keys", theme, box, 300);
  await context.close();

  ({ context, page } = await open({ width: 1000, height: 1000 }, 2.5, freshJar, "http://127.0.0.1:8722"));
  await page.goto("http://127.0.0.1:8722/", { waitUntil: "networkidle" });
  box = await page.getByRole("heading", { name: "Or pick an agent" }).locator("xpath=following-sibling::div[1]").boundingBox();
  await crop(page, "agents", theme, box);
  await context.close();
}
await browser.close();
console.log(`the screens are in ${out}`);
finish(0);
