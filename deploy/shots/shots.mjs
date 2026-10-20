// Photographs every page of the app against a seeded local server, in light and dark, at desktop and phone width, so a change to
// how the pages look can be seen on all of them at once.
//
//   cd <a folder with playwright installed> && node /path/to/deploy/shots/shots.mjs <out dir> [route filter]
//
// It builds ./cmd/gusto, starts a stub model and a server on 127.0.0.1:8700 whose agent is a canned one that asks the model
// through the proxy and writes a file, makes a few conversations, a routine, a checked task and a signed record, then visits each
// route. A page's name and its screenshots land in <out dir>/<theme>-<width>/<name>.png, and statuses and console errors in
// <out dir>/report.txt.
import { spawn, execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync, rmSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { chromium } from "playwright";

const root = resolve(new URL("../..", import.meta.url).pathname);
const out = resolve(process.argv[2] ?? "shots-out");
const only = process.argv[3];
const port = 8700, base = `http://127.0.0.1:${port}`, password = "shots";
const work = mkdtempSync(join(tmpdir(), "gusto-shots-"));
const children = [];

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
function start(cmd, args, env = {}) {
  const child = spawn(cmd, args, { env: { ...process.env, ...env }, stdio: ["ignore", "pipe", "pipe"] });
  child.stdout.on("data", (d) => writeFileSync(join(work, "log"), d, { flag: "a" }));
  child.stderr.on("data", (d) => writeFileSync(join(work, "log"), d, { flag: "a" }));
  children.push(child);
  return child;
}
function finish(code) {
  for (const child of children) child.kill("SIGTERM");
  rmSync(work, { recursive: true, force: true });
  process.exit(code);
}
process.on("SIGINT", () => finish(1));

// 1. the binary, the stub model, the keys and the canned agent
const bin = join(work, "gusto");
execFileSync("go", ["build", "-o", bin, "./cmd/gusto"], { cwd: root, env: { ...process.env, CC: process.env.CC ?? "/usr/bin/clang" }, stdio: "inherit" });
mkdirSync(join(work, "creds"));
mkdirSync(join(work, "agents"));
mkdirSync(join(work, "keys"));
writeFileSync(join(work, "creds", "stub.json"), JSON.stringify({ address: "http://127.0.0.1:9000/v1", auth: "bearer", key: "stub", wires: ["chat"] }));
writeFileSync(join(work, "spend.json"), "{}");
const ask = "fetch(process.env.GUSTO_MODEL_URL+'/chat/completions',{method:'POST',headers:{'content-type':'application/json',authorization:'Bearer '+process.env.GUSTO_MODEL_TOKEN},body:JSON.stringify({model:'stub-model',messages:[{role:'user',content:'hi'}]})}).then(r=>r.json()).then(j=>console.log(j.choices[0].message.content))";
writeFileSync(join(work, "agents", "demo.json"), JSON.stringify({
  name: "demo", title: "OpenCode", what: "A canned agent for screenshots: asks the model once and writes a file.",
  program: "sh",
  argv: ["-c", `node -e "${ask}" && printf '# Spending policy\\n\\n- $50 a month per person\\n- Reviewed each quarter\\n\\n1. Ask first\\n2. Spend second\\n' > policy.md`],
  env: { GUSTO_MODEL_URL: "{modelUrl}", GUSTO_MODEL_TOKEN: "{modelToken}" }, needs: "key", stream: "plain",
}));
mkdirSync(join(work, "retention"));
writeFileSync(join(work, "retention", "policy.json"), JSON.stringify({ workspaceDays: { passed: 30, failed: 90 }, recordDays: { passed: 365 } }));
const ago = (days) => new Date(Date.now() - days * 864e5).toISOString();
writeFileSync(join(work, "members.json"), JSON.stringify({
  written: ago(0),
  members: [{ email: "ada@example.com", role: "admin", joined: ago(40), lastSignIn: ago(0) }, { email: "grace@example.com", role: "runner", joined: ago(12), lastSignIn: ago(2) }],
  invited: [{ email: "linus@example.com", by: "ada@example.com", created: ago(1), expires: ago(-6) }],
}));
execFileSync(bin, ["keygen", "shots", "-dir", join(work, "keys")], { stdio: "ignore" });
start("python3", [join(root, "deploy/stubmodel/stub.py")]);
const server = start(bin, ["serve"], {
  GUSTO_PASSWORD: password, GUSTO_ADDR: `127.0.0.1:${port}`, GUSTO_JOBS: join(work, "jobs"), GUSTO_SESSIONS: join(work, "sessions"),
  GUSTO_SANDBOX: "here", GUSTO_CREDENTIALS: join(work, "creds"), GUSTO_MODELS: "stub/stub-model", GUSTO_HARNESSES: join(work, "agents"),
  GUSTO_SIGNING_KEY: join(work, "keys", "shots.key"), GUSTO_KEYRING: join(work, "keys"), GUSTO_NOTIFY_WEBHOOK: "", HOME: work,
  GUSTO_RETENTION: join(work, "retention"), GUSTO_API_TOKENS: join(work, "tokens.json"), GUSTO_MEMBERS: join(work, "members.json"),
});
for (let i = 0; ; i++) {
  try { if ((await fetch(`${base}/healthz`)).ok) break; } catch {}
  if (i > 60) { console.error("the server did not start:\n" + (await import("node:fs")).readFileSync(join(work, "log"), "utf8").slice(-1500)); finish(1); }
  await sleep(500);
}

// 2. sign in and seed
const jar = {};
const call = async (method, path, form) => {
  const res = await fetch(base + path, {
    method, redirect: "manual",
    headers: { cookie: Object.entries(jar).map(([k, v]) => `${k}=${v}`).join("; "), ...(form ? { "content-type": "application/x-www-form-urlencoded" } : {}) },
    body: form ? new URLSearchParams(form).toString() : undefined,
  });
  for (const line of res.headers.getSetCookie?.() ?? []) { const [pair] = line.split(";"); const [k, v] = pair.split("="); jar[k] = v; }
  return res;
};
await call("POST", "/login", { password });
const started = await call("POST", "/instances", { harness: "demo", model: "stub/stub-model" });
const thread = started.headers.get("location");
const instance = thread.split("/threads/")[0];
const said = async (path, words) => {
  const res = await call("POST", `${path}/say`, { said: words });
  return (res.headers.get("location") ?? "").split("#")[1];
};
const turns = [await said(thread, "What should a 40-person company decide first about AI agents?"),
  await said(thread, "Draft a spending policy with a monthly limit per person and save it as policy.md.")];
const second = (await call("POST", "/instances", { harness: "demo", model: "stub/stub-model" })).headers.get("location");
await said(second, "Summarize what changed this week.");
const task = (await call("POST", `${instance}/tasks`, { brief: "Write the policy and check it exists.", gate: "true", team: "finance", usd: "1.00" })).headers.get("location");
await call("POST", `${instance}/routines`, { title: "Morning summary", prompt: "Summarize what changed since yesterday.", daily: "08:00" });
await call("POST", `${instance}/routines`, { title: "Spend check", prompt: "How much did the team spend this week?", every: "6" });
for (let i = 0; i < 90; i++) {
  const page = await (await call("GET", "/jobs")).text();
  if (!/queued|running/i.test(page.replace(/<[^>]*>/g, " ").match(/Queued\s*\d+|Running\s*\d+/g)?.join(" ") ?? "") && i > 4) break;
  await sleep(1000);
}
await sleep(4000);

// 3. the routes
const jobId = turns[1];
const routes = [
  ["login", "/login", false], ["home", "/"], ["chat", "/chat"], ["new", "/new"], ["engines", "/engines"], ["jobs", "/jobs"],
  ["spend", "/spend"], ["usage", "/usage"], ["files", "/files"], ["setups", "/setups"], ["start-agent", "/instances/new?harness=demo"],
  ["instance", instance], ["thread", thread], ["record", `/jobs/${jobId}`], ["task", task ?? `/jobs/${jobId}`],
  ["file", `${instance}/file?path=policy.md`], ["folder", `${instance}/files`],
  ["admin", "/admin"], ["admin-spending", "/admin/spending"], ["admin-usage", "/admin/usage"], ["admin-activity", "/admin/activity"],
  ["admin-people", "/admin/people"], ["admin-models", "/admin/models"], ["admin-retention", "/admin/retention"], ["admin-tokens", "/admin/tokens"],
  ["admin-skill-sets", "/admin/skill-sets"],
].filter(([name]) => !only || name.includes(only));

// 4. the photographs
rmSync(out, { recursive: true, force: true });
const browser = await chromium.launch();
const report = [];
for (const theme of ["light", "dark"]) {
  for (const [label, size] of [["desktop", { width: 1280, height: 800 }], ["phone", { width: 390, height: 844 }]]) {
    const context = await browser.newContext({ viewport: size, deviceScaleFactor: 1, colorScheme: theme });
    await context.addCookies(Object.entries(jar).map(([name, value]) => ({ name, value, url: base })));
    await context.addInitScript((t) => { try { localStorage.setItem("gusto-theme", t); } catch {} }, theme);
    const dir = join(out, `${theme}-${label}`);
    mkdirSync(dir, { recursive: true });
    for (const [name, path, signedIn = true] of routes) {
      const page = await context.newPage();
      const errors = [];
      page.on("console", (m) => { if (m.type() === "error") errors.push(m.text()); });
      page.on("pageerror", (e) => errors.push(String(e)));
      if (!signedIn) await context.clearCookies();
      const res = await page.goto(base + path, { waitUntil: "networkidle" }).catch((e) => ({ status: () => `ERR ${e.message}` }));
      await page.waitForTimeout(250);
      await page.screenshot({ path: join(dir, `${name}.png`), fullPage: true });
      report.push(`${theme}-${label} ${name} ${res.status?.()} ${errors.length ? "console: " + errors.join(" | ").slice(0, 200) : ""}`);
      await page.close();
      if (!signedIn) await context.addCookies(Object.entries(jar).map(([n, value]) => ({ name: n, value, url: base })));
    }
    await context.close();
  }
}
await browser.close();
writeFileSync(join(out, "report.txt"), report.join("\n") + "\n");
console.log(`${routes.length * 4} screenshots in ${out}`);
finish(0);
