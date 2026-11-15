// Starts a real gusto for the end-to-end tests, as Playwright's webServer: it builds the web app into the binary's embedded
// files and builds ./cmd/gusto, then starts a stub model, an identity provider and `gusto serve` on fixed ports, with everything
// it keeps in a temporary folder that goes when it stops.
//
//   node e2e/server.mjs      (from web/; Playwright runs it)
//
// The agent is a canned one: it asks the model once through gusto's proxy, then writes policy.md, so a reply has a file. The
// server's own log is kept in e2e/.run/server.log for when a test fails.
import { execFileSync, spawn } from "node:child_process"
import {
  createWriteStream,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs"
import { connect, createServer } from "node:net"
import { tmpdir } from "node:os"
import { join, resolve } from "node:path"

import { startIdentity } from "./identity.mjs"
import {
  admin,
  client,
  gustoPort,
  gustoURL,
  identityPort,
  modelPort,
  proxyPort,
  runner,
  sharedPassword,
} from "./values.mjs"

const web = resolve(import.meta.dirname, "..")
const root = resolve(web, "..")
const run = join(web, "e2e", ".run")
mkdirSync(run, { recursive: true })
const log = createWriteStream(join(run, "server.log"))
const work = mkdtempSync(join(tmpdir(), "gusto-e2e-"))
const children = []
const closers = []

function finish(code) {
  for (const child of children) child.kill("SIGTERM")
  for (const close of closers) close()
  rmSync(work, { recursive: true, force: true })
  process.exit(code)
}
process.on("SIGINT", () => finish(0))
process.on("SIGTERM", () => finish(0))

// taken says whether something answers on the port or holds it, so a server left from another run, or another project's, is
// named rather than tested by mistake.
function taken(port) {
  return new Promise((done) => {
    const knock = connect(port, "127.0.0.1")
    knock.once("connect", () => {
      knock.destroy()
      done(true)
    })
    knock.once("error", () => {
      const probe = createServer()
      probe.once("error", () => done(true))
      probe.listen(port, "127.0.0.1", () => probe.close(() => done(false)))
    })
  })
}

for (const port of [gustoPort, identityPort, modelPort, proxyPort]) {
  if (await taken(port)) {
    console.error(
      `e2e: port ${port} is taken; stop what listens on it (lsof -i :${port}) and run again`
    )
    finish(1)
  }
}

// start runs a program whose output goes to the log, and ends the run when it stops by itself.
function start(name, command, args, env = {}) {
  const child = spawn(command, args, {
    env: { ...process.env, ...env },
    stdio: ["ignore", "pipe", "pipe"],
  })
  child.stdout.on("data", (data) => log.write(data))
  child.stderr.on("data", (data) => log.write(data))
  child.on("exit", (code, signal) => {
    console.error(
      `e2e: ${name} stopped (${signal ?? code}); its output, and gusto's, is in e2e/.run/server.log`
    )
    finish(1)
  })
  children.push(child)
}

// The binary carries the web app it was built with, so the app is built from this source first.
execFileSync("npx", ["vite", "build", "--logLevel", "warn"], {
  cwd: web,
  stdio: "inherit",
})
const bin = join(work, "gusto")
execFileSync("go", ["build", "-o", bin, "./cmd/gusto"], {
  cwd: root,
  env: { ...process.env, CC: process.env.CC ?? "/usr/bin/clang" },
  stdio: "inherit",
})

for (const dir of ["creds", "agents", "keys", "retention"])
  mkdirSync(join(work, dir))
writeFileSync(
  join(work, "creds", "stub.json"),
  JSON.stringify({
    address: `http://127.0.0.1:${modelPort}/v1`,
    auth: "bearer",
    key: "stub",
    wires: ["chat"],
  })
)
const ask =
  "fetch(process.env.GUSTO_MODEL_URL+'/chat/completions',{method:'POST',headers:{'content-type':'application/json',authorization:'Bearer '+process.env.GUSTO_MODEL_TOKEN},body:JSON.stringify({model:'stub-model',messages:[{role:'user',content:'hi'}]})}).then(r=>r.json()).then(j=>console.log(j.choices[0].message.content))"
writeFileSync(
  join(work, "agents", "opencode.json"),
  JSON.stringify({
    name: "opencode",
    title: "OpenCode",
    what: "A canned agent for the tests: asks the model once and writes a file.",
    program: "sh",
    argv: [
      "-c",
      `node -e "${ask}" && printf '# Spending policy\\n\\n- $50 a month per person\\n' > policy.md`,
    ],
    env: { GUSTO_MODEL_URL: "{modelUrl}", GUSTO_MODEL_TOKEN: "{modelToken}" },
    needs: "key",
    stream: "plain",
    // It takes skills, so a template's skill set can be started on it.
    skills: "/root/.agents/skills",
  })
)
writeFileSync(
  join(work, "retention", "policy.json"),
  JSON.stringify({
    workspaceDays: { passed: 30, failed: 90 },
    recordDays: { passed: 365 },
  })
)
// The stub model has a price, so a budget can count what an agent spends: under a budget, a call to a model with no price is
// refused, since the budget could not count it.
writeFileSync(
  join(work, "spend.json"),
  JSON.stringify({
    prices: { "stub-model": { inputPerMillion: 1, outputPerMillion: 1 } },
  })
)
const now = new Date().toISOString()
writeFileSync(
  join(work, "members.json"),
  JSON.stringify({
    written: now,
    members: [admin, runner].map((person) => ({
      email: person.email,
      role: person.role,
      joined: now,
    })),
    invited: [],
  })
)
execFileSync(bin, ["keygen", "e2e", "-dir", join(work, "keys")], {
  stdio: "ignore",
})

const redirect = `${gustoURL}/auth/callback`
const identity = await startIdentity({ port: identityPort, client, redirect })
closers.push(identity.close)
start("the stub model", "python3", [join(root, "deploy/stubmodel/stub.py")], {
  STUB_PORT: String(modelPort),
})
start("gusto", bin, ["serve"], {
  HOME: work,
  GUSTO_ADDR: `127.0.0.1:${gustoPort}`,
  GUSTO_PASSWORD: sharedPassword,
  GUSTO_PROXY_ADDR: `127.0.0.1:${proxyPort}`,
  GUSTO_OIDC_ISSUER: identity.issuer,
  GUSTO_OIDC_CLIENT_ID: client.id,
  GUSTO_OIDC_CLIENT_SECRET: client.secret,
  GUSTO_OIDC_REDIRECT: redirect,
  GUSTO_ALLOWED: [admin.email, runner.email].join(","),
  GUSTO_ADMINS: admin.email,
  GUSTO_MEMBERS: join(work, "members.json"),
  GUSTO_TASKS: join(work, "jobs"),
  GUSTO_SESSIONS: join(work, "sessions"),
  GUSTO_SANDBOX: "here",
  GUSTO_CREDENTIALS: join(work, "creds"),
  GUSTO_MODELS: "stub/stub-model",
  GUSTO_SPEND_POLICY: join(work, "spend.json"),
  GUSTO_HARNESSES: join(work, "agents"),
  GUSTO_SIGNING_KEY: join(work, "keys", "e2e.key"),
  GUSTO_KEYRING: join(work, "keys"),
  GUSTO_NOTIFY_WEBHOOK: "",
  GUSTO_RETENTION: join(work, "retention"),
  GUSTO_ORG_SETTINGS: join(work, "org-settings.json"),
  GUSTO_API_TOKENS: join(work, "tokens.json"),
  GUSTO_SETUPS: join(work, "setups"),
  GUSTO_COMPARISONS: join(work, "comparisons"),
  GUSTO_CHANGES: join(work, "changes", "changes.jsonl"),
  GUSTO_INTEGRATIONS: join(work, "integrations"),
  GUSTO_PRESETS: join(work, "presets"),
  GUSTO_CREW: join(work, "crew"),
})
console.log(
  `e2e: gusto is starting at ${gustoURL}; its log is e2e/.run/server.log`
)
