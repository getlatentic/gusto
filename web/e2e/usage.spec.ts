import type { Page } from "@playwright/test"

import { expect, Gusto, test, unique } from "./fixtures.ts"
import { connectedApp, runner } from "./values.mjs"

// The pages of a person's own: their usage and its summary, the apps they connect, and the computers they approve. Each journey
// is the runner's, since what a person sees of their own is the point, and the admin would see everyone's.

// talked has the runner ask their own agent something in a room of theirs, and waits for the reply, so the usage holds a
// conversation whose calls name that agent.
async function talked(runnerGusto: Gusto): Promise<string> {
  const agent = unique("Scout")
  const id = await runnerGusto.makeAgent(agent)
  const room = await runnerGusto.makeRoom(unique("Ledger "))
  await runnerGusto.addToRoom(room, { agents: [id] })
  const turn = await runnerGusto.say(room.path, `@${agent} write the policy`)
  await runnerGusto.replied(room.path, turn)
  return agent
}

// sideways is how far the page reaches past the window's width, which a phone would scroll sideways for.
const sideways = (page: Page) =>
  page.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement
    return root.scrollWidth - root.clientWidth
  })

test.describe("a person's usage", () => {
  test("after a conversation, their usage lists it and opens its session", async ({
    runnerGusto,
    runnerPage: page,
  }) => {
    const agent = await talked(runnerGusto)
    // The index takes in a call shortly after it is made, so the page is read again until it is there.
    await expect(async () => {
      await page.goto(`/usage?q=${encodeURIComponent(agent)}`)
      await expect(page.getByTestId("usage-row")).toHaveCount(1, {
        timeout: 2_000,
      })
    }).toPass()
    await expect(page.getByTestId("page-title")).toHaveText("Usage")
    await expect(
      page.getByRole("columnheader", { name: "Asked by" })
    ).toHaveCount(0)
    await expect(page.getByRole("button", { name: "Person" })).toHaveCount(0)
    await expect(page.getByTestId("usage-row")).toContainText(agent)

    await page.getByTestId("usage-row").getByRole("link").first().click()
    await expect(page).toHaveURL(/\/usage\/[^/?]+/)
    await expect(page.getByTestId("page-title")).toContainText("Session")
    await expect(page.getByText(`agent ${agent}`)).toBeVisible()
    await expect(page.getByTestId("session-call").first()).toBeVisible()
    await expect(
      page
        .getByRole("navigation", { name: "Where this is" })
        .getByRole("link", { name: "Usage" })
    ).toHaveAttribute("href", "/usage")
  })

  test("their summary shows the month's figures", async ({
    runnerGusto,
    runnerPage: page,
  }) => {
    await talked(runnerGusto)
    await page.goto("/spend")
    await expect(page.getByTestId("page-title")).toHaveText("Usage")
    await expect(
      page.getByRole("tab", { name: "Summary", selected: true })
    ).toBeVisible()
    await expect(
      page.getByTestId("spend-calls").getByTestId("figure-value")
    ).not.toHaveText("0")
    await expect(
      page.getByTestId("spend-spent").getByTestId("figure-value")
    ).toHaveText(/^\$\d/)
    await expect(page.getByRole("combobox", { name: "Month" })).toContainText(
      new Date().toISOString().slice(0, 7)
    )
    await page.getByRole("tab", { name: "Conversations" }).click()
    await expect(page).toHaveURL(/\/usage$/)
  })
})

test.describe("connections", () => {
  test("a person connects the app an admin turned on, comes back told so, chooses it for their agents and disconnects", async ({
    gusto,
    runnerPage: page,
  }) => {
    await gusto.post(`/admin/connectors/${connectedApp.id}/on`, {})
    await page.goto("/connections")
    await expect(page.getByTestId("page-title")).toHaveText("Connections")
    const row = page.locator(
      `[data-testid="connection"][data-app="${connectedApp.id}"]`
    )
    await expect(row).toContainText(connectedApp.name)
    await expect(row).toContainText("not yet")

    // Connecting leaves for the app's own sign-in page, and the app sends the person back.
    await row.getByTestId("connect").click()
    await page.getByTestId("app-allow").click()
    await expect(page).toHaveURL(/\/connections\?connected=/)
    await expect(
      page.getByText(
        `You are signed in to ${connectedApp.name}. Your agents use it from their next task.`
      )
    ).toBeVisible()
    await expect(row.getByTestId("connected-since")).toHaveText("just now")

    const choice = row.getByTestId("for-agents")
    await expect(choice).toContainText("Off")
    await choice.click()
    await page.getByRole("option", { name: "On" }).click()
    await expect(row.getByTestId("for-agents")).toContainText("On")
    await page.reload()
    await expect(row.getByTestId("for-agents")).toContainText("On")

    await row.getByTestId("disconnect").click()
    await expect(row).toContainText("not yet")
    await expect(row.getByTestId("connect")).toBeVisible()
  })
})

test.describe("your computers", () => {
  test("a computer asks to sign in, the person approves its code, the computer is given its token, and the person ends it", async ({
    playwright,
    baseURL,
    runnerPage: page,
  }) => {
    const computer = await playwright.request.newContext({ baseURL })
    const label = unique("laptop-")
    const started = await (
      await computer.post("/api/device", { data: { label } })
    ).json()
    expect(started.verification_uri).toMatch(/\/device$/)
    const asked = await computer.post("/api/device/token", {
      data: { device_code: started.device_code },
    })
    expect((await asked.json()).error).toBe("authorization_pending")

    await page.goto(`/device?code=${started.user_code}`)
    await expect(page.getByTestId("page-title")).toHaveText("Your computers")
    await expect(page.getByLabel("The code your computer shows")).toHaveValue(
      started.user_code
    )
    await page.getByRole("button", { name: "Approve" }).click()
    await expect(page.getByTestId("device-approved")).toHaveText(
      `Approved ${label}. Go back to the computer: it signs in on its own.`
    )
    await expect(page.getByLabel("The code your computer shows")).toHaveValue(
      ""
    )

    const given = await computer.post("/api/device/token", {
      data: { device_code: started.device_code },
    })
    expect(given.status()).toBe(200)
    const token = (await given.json()) as {
      access_token: string
      person: string
    }
    expect(token.person).toBe(runner.email)
    const me = await computer.get("/api/me", {
      headers: { authorization: `Bearer ${token.access_token}` },
    })
    expect(me.status()).toBe(200)

    await page.reload()
    const listed = page
      .getByTestId("computer")
      .filter({ hasText: label.toLowerCase() })
    await expect(listed).toHaveCount(1)
    await listed.getByRole("button", { name: "End" }).click()
    await expect(listed).toContainText("ended")
    const after = await computer.get("/api/me", {
      headers: { authorization: `Bearer ${token.access_token}` },
    })
    expect(after.status()).toBe(401)
    await computer.dispose()
  })

  test("a code that is not waiting is refused, and says why", async ({
    runnerPage: page,
  }) => {
    await page.goto("/device")
    await page.getByLabel("The code your computer shows").fill("BCDF-GHJK")
    await page.getByRole("button", { name: "Approve" }).click()
    await expect(page.getByRole("alert")).toHaveText(
      "That code is not waiting. Start the sign-in again on your computer and use the new code."
    )
  })
})

test.describe("at a phone's width", () => {
  test.use({ viewport: { width: 375, height: 812 }, isMobile: true })

  for (const path of ["/usage", "/spend", "/connections", "/device"]) {
    test(`${path} does not scroll sideways`, async ({ page, gusto }) => {
      await gusto.post(`/admin/connectors/${connectedApp.id}/on`, {})
      await page.goto(path)
      await expect(page.getByTestId("page-title")).toBeVisible()
      await expect(page.locator('[aria-busy="false"]').first()).toBeVisible()
      expect(await sideways(page)).toBe(0)
    })
  }
})
