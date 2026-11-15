import type { Page } from "@playwright/test"

import { expect, roomWithAgent, test } from "./fixtures.ts"

// sideways is how far the page reaches past the window's width, which a phone would scroll sideways for.
const sideways = (page: Page) =>
  page.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement
    return root.scrollWidth - root.clientWidth
  })

test.describe("at a phone's width", () => {
  test.use({ viewport: { width: 375, height: 812 }, isMobile: true })

  for (const [name, path] of [
    ["the new chat page", "/chat"],
    ["the console's overview", "/admin"],
    ["the console's budgets", "/admin/budgets"],
  ]) {
    test(`${name} does not scroll sideways`, async ({ page }) => {
      await page.goto(path)
      await expect(page.getByTestId("page-title")).toBeVisible()
      expect(await sideways(page)).toBe(0)
    })
  }

  test("a conversation does not scroll sideways", async ({ page, gusto }) => {
    const { room, agent } = await roomWithAgent(gusto)
    const turn = await gusto.say(room.path, `@${agent} write the policy`)
    await gusto.replied(room.path, turn)
    await page.goto(room.path)
    await expect(page.getByTestId("reply")).toHaveAttribute("data-done", "")
    expect(await sideways(page)).toBe(0)
  })
})
