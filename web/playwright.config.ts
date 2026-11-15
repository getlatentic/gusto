import { defineConfig, devices } from "@playwright/test"

import { admin, gustoURL, stateOf } from "./e2e/values.mjs"

// The end-to-end tests drive a real gusto, which e2e/server.mjs builds and starts, in Chromium. The setup project signs each
// person in once and keeps their browser state; every test starts signed in as the admin, and a test that needs the runner too
// opens a second context with theirs.
export default defineConfig({
  testDir: "e2e",
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  // One server serves every worker, and a reply runs one agent at a time, so a few workers keep the waits short.
  workers: 3,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: gustoURL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    { name: "setup", testMatch: /sign-in\.setup\.ts/ },
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"], storageState: stateOf(admin) },
      dependencies: ["setup"],
    },
  ],
  webServer: {
    command: "node e2e/server.mjs",
    url: `${gustoURL}/healthz`,
    // The first run builds the web app and the gusto binary.
    timeout: 300_000,
    reuseExistingServer: false,
    stdout: "pipe",
    stderr: "pipe",
    gracefulShutdown: { signal: "SIGTERM", timeout: 5_000 },
  },
})
