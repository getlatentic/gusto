// Retakes the landing page's hero screens from the current app, at twice the pixel density, in light and dark.
//
//   cd <a folder with playwright installed> && node /path/to/deploy/landing/capture.mjs <out dir>
//
// It starts the server the way deploy/shots/shots.mjs does, with a model that replays the words of a real demo run, has
// the finance conversation, and photographs the chat. <out dir>/hero-light.png and hero-dark.png are then encoded with
// cwebp into internal/cloud/static/.
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
const start = (cmd, args, env = {}) => { const c = spawn(cmd, args, { env: { ...process.env, ...env }, stdio: "ignore" }); children.push(c); return c; };
const finish = (code) => { children.forEach((c) => c.kill("SIGTERM")); rmSync(work, { recursive: true, force: true }); process.exit(code); };

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
writeFileSync(join(work, "replies.json"), JSON.stringify(turns));
writeFileSync(join(work, "creds", "gemma.json"), JSON.stringify({ address: "http://127.0.0.1:9100/v1", auth: "bearer", key: "x", wires: ["chat"] }));
writeFileSync(join(work, "spend.json"), JSON.stringify({ prices: { "gemma-demo": { inputPerMillion: 0.13, outputPerMillion: 0.4 } }, teams: {} }));
const ask = "fetch(process.env.GUSTO_MODEL_URL+'/chat/completions',{method:'POST',headers:{'content-type':'application/json',authorization:'Bearer '+process.env.GUSTO_MODEL_TOKEN},body:JSON.stringify({model:'gemma-demo',messages:[{role:'user',content:'hi'}]})}).then(r=>r.json()).then(j=>console.log(j.choices[0].message.content))";
writeFileSync(join(work, "agents", "opencode.json"), JSON.stringify({
  name: "opencode", title: "OpenCode", what: "Open source, and points at whichever model the deployment gives it.", program: "sh",
  argv: ["-c", `node -e "${ask}" | tee out.txt; grep -q 'monthly limit' out.txt && printf '# Spending policy\\n\\n- $50 a month per person\\n' > policy.md; rm -f out.txt`],
  env: { GUSTO_MODEL_URL: "{modelUrl}", GUSTO_MODEL_TOKEN: "{modelToken}" }, needs: "key", stream: "plain",
}));
execFileSync(bin, ["keygen", "capture", "-dir", join(work, "keys")], { stdio: "ignore" });
start("python3", [join(root, "deploy/landing/replay.py"), join(work, "replies.json")]);
start(bin, ["serve"], {
  GUSTO_PASSWORD: password, GUSTO_ADDR: `127.0.0.1:${port}`, GUSTO_JOBS: join(work, "jobs"), GUSTO_SESSIONS: join(work, "sessions"),
  GUSTO_SANDBOX: "here", GUSTO_CREDENTIALS: join(work, "creds"), GUSTO_MODELS: "gemma/gemma-demo", GUSTO_HARNESSES: join(work, "agents"),
  GUSTO_SIGNING_KEY: join(work, "keys", "capture.key"), GUSTO_KEYRING: join(work, "keys"), GUSTO_SPEND_POLICY: join(work, "spend.json"),
  GUSTO_PROXY_ADDR: "127.0.0.1:8721", HOME: work,
});
for (let i = 0; ; i++) {
  try { if ((await fetch(`${base}/healthz`)).ok) break; } catch {}
  if (i > 60) { console.error("the server did not start"); finish(1); }
  await sleep(500);
}

const jar = {};
const call = async (method, path, form) => {
  const res = await fetch(base + path, { method, redirect: "manual",
    headers: { cookie: Object.entries(jar).map(([k, v]) => `${k}=${v}`).join("; "), ...(form ? { "content-type": "application/x-www-form-urlencoded" } : {}) },
    body: form ? new URLSearchParams(form).toString() : undefined });
  for (const line of res.headers.getSetCookie?.() ?? []) { const [pair] = line.split(";"); const [k, v] = pair.split("="); jar[k] = v; }
  return res;
};
await call("POST", "/login", { password });
const thread = (await call("POST", "/instances", { harness: "opencode", model: "gemma/gemma-demo" })).headers.get("location");
for (const turn of turns) {
  await call("POST", `${thread}/say`, { said: turn.say });
  for (let i = 0; i < 60; i++) { await sleep(1000); const page = await (await call("GET", thread)).text(); if (!/Working|Waiting/.test(page) && page.includes(turn.say.slice(0, 20))) { await sleep(1500); break; } }
}

rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });
const browser = await chromium.launch();
for (const theme of ["light", "dark"]) {
  const context = await browser.newContext({ viewport: { width: 1000, height: 1000 }, deviceScaleFactor: 2, colorScheme: theme });
  await context.addCookies(Object.entries(jar).map(([name, value]) => ({ name, value, url: base })));
  await context.addInitScript((t) => { try { localStorage.setItem("gusto-theme", t); } catch {} }, theme);
  const page = await context.newPage();
  await page.goto(base + thread, { waitUntil: "networkidle" });
  await page.evaluate(() => { const org = document.querySelector("#sidepanel span"); if (org) org.textContent = "demo-relay-labs-89ec"; });
  await page.evaluate(() => { const said = document.getElementById("said"); if (said) said.scrollTop = 0; });
  await page.waitForTimeout(400);
  await page.screenshot({ path: join(out, `hero-${theme}.png`), clip: { x: 0, y: 0, width: 1000, height: 680 } });
  await context.close();
}
await browser.close();
console.log(`hero-light.png and hero-dark.png are in ${out}`);
finish(0);
