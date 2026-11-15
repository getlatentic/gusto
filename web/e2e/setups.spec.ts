import type { Page } from "@playwright/test"

import { expect, test, unique } from "./fixtures.ts"

// The pages a person starts an agent from: their presets, the built-in ones, a template's page and a skill set's.

const conversation = /\/instances\/[^/]+\/threads\/[^/#?]+$/

// sideways is how far the page reaches past the window's width, which a phone would scroll sideways for.
const sideways = (page: Page) =>
  page.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement
    return root.scrollWidth - root.clientWidth
  })

test.describe("starting from a preset or a skill set", () => {
  test("a preset saved from the chat is listed on the presets page, starts a new chat, and is removed", async ({
    page,
  }) => {
    const name = unique("Reviewer ")
    await page.goto("/chat")
    await page.getByTestId("run-on").click()
    await page.getByTestId("choose-preset").click()
    await page
      .getByTestId("choice-option")
      .filter({ hasText: "Save this setup as a preset…" })
      .click()
    await page.getByLabel("Name of the preset").fill(name)
    await page.getByRole("button", { name: "Save preset" }).click()
    await expect(page).toHaveURL(/\/chat\?preset=/)

    await page.goto("/presets")
    const row = page.getByTestId("preset-row").filter({ hasText: name })
    await expect(row).toContainText("with Stub Model")
    await row.getByRole("link", { name: "New chat" }).click()
    await expect(page).toHaveURL(/\/chat\?preset=/)
    await page.getByTestId("run-on").click()
    await expect(page.getByTestId("choose-preset")).toContainText(name)

    await page.goto("/presets")
    await row.getByRole("button", { name: `Remove ${name}` }).click()
    await expect(page).toHaveURL(/\/presets$/)
    await expect(page.getByTestId("page-title")).toHaveText("Presets")
    await expect(row).toHaveCount(0)
  })

  test("a built-in preset opens its template, which starts an agent in a conversation", async ({
    page,
  }) => {
    await page.goto("/presets")
    await page
      .getByTestId("pick-role")
      .filter({ hasText: "Clinical documentation" })
      .click()
    const detail = page.getByTestId("role-detail")
    await expect(detail).toContainText("Try asking")
    await detail.getByTestId("role-details").click()

    await expect(page).toHaveURL(/\/templates\/health$/)
    await expect(page.getByTestId("page-title")).toHaveText(
      "Clinical documentation"
    )
    await expect(page.getByText("What it can do")).toBeVisible()
    await page.getByTestId("start-template").click()
    await expect(page).toHaveURL(conversation)
    await expect(page.getByTestId("conversation-title")).toBeVisible()
  })

  test("a skill set opens from the list, and starts an agent with it", async ({
    page,
    gusto,
  }) => {
    // A template's skills are a skill set once it has started an agent.
    await gusto.post("/templates/health", { harness: "opencode" })
    await page.goto("/setups")
    await page.getByTestId("setup-link").filter({ hasText: "health" }).click()

    await expect(page).toHaveURL(/\/setups\/health$/)
    await expect(page.getByTestId("page-title")).toHaveText("health")
    await expect(page.getByText("phi-guard")).toBeVisible()
    await page.getByTestId("start-with").filter({ hasText: "opencode" }).click()

    await expect(page).toHaveURL(
      /\/instances\/new\?harness=opencode&setup=health$/
    )
    await expect(page.getByTestId("page-title")).toHaveText("OpenCode")
    await expect(page.getByTestId("set-up-form")).toContainText("health")
    await page.getByTestId("start-set-up").click()
    await expect(page).toHaveURL(conversation)
    await expect(page.getByTestId("conversation-title")).toBeVisible()
  })

  test("an agent of one's own starts from the blank page", async ({ page }) => {
    await page.goto("/new/blank")
    await page.getByLabel("Name it").fill(unique("Helper "))
    await page.getByTestId("start-blank").click()
    await expect(page).toHaveURL(conversation)
  })
})

test.describe("the start pages at a phone's width", () => {
  test.use({ viewport: { width: 375, height: 812 }, isMobile: true })

  for (const path of [
    "/presets",
    "/setups",
    "/templates/health",
    "/new/blank",
    "/instances/new?harness=opencode",
  ]) {
    test(`${path} does not scroll sideways`, async ({ page }) => {
      await page.goto(path)
      await expect(page.getByTestId("page-title")).toBeVisible()
      await expect(page.locator('[aria-busy="false"]')).toBeVisible()
      expect(await sideways(page)).toBe(0)
    })
  }

  test("the presets are one menu at a phone's width", async ({ page }) => {
    await page.goto("/presets")
    await expect(page.getByLabel("Built-in preset")).toBeVisible()
    await expect(page.getByTestId("role-search")).toBeHidden()
  })
})

test.describe("signed out", () => {
  test.use({ storageState: { cookies: [], origins: [] } })

  test("the sign-in page asks for nothing a session gives", async ({
    page,
  }) => {
    const asked: string[] = []
    page.on("request", (request) => asked.push(new URL(request.url()).pathname))
    await page.goto("/login")
    await expect(page.getByTestId("provider-sign-in")).toBeVisible()
    expect(asked.filter((path) => path.startsWith("/app/api/"))).toEqual([
      "/app/api/login",
    ])
  })

  test("the sign-in page does not scroll sideways at a phone's width", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 375, height: 812 })
    await page.goto("/login?shared")
    await expect(page.getByTestId("shared-password")).toBeVisible()
    expect(await sideways(page)).toBe(0)
  })
})
