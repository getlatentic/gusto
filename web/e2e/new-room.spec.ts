import { expect, test, unique } from "./fixtures.ts"

test.describe("making a room", () => {
  test("a named room opens with its dialog for adding members, and is in the side list", async ({
    page,
  }) => {
    await page.goto("/rooms/new")
    await page.getByTestId("room-name").fill(unique("Launch "))
    await page.getByTestId("make-room").click()

    await expect(page).toHaveURL(/\/instances\/[^/]+\/threads\/[^/]+\?add=1$/)
    await expect(page.getByTestId("add-dialog")).toBeVisible()
    const path = new URL(page.url()).pathname
    await expect(
      page
        .getByTestId("side-rooms")
        .locator(`[data-testid="side-row"][href="${path}"]`)
    ).toBeVisible()
  })

  test("a name of only spaces is refused, and the page says why", async ({
    page,
  }) => {
    await page.goto("/rooms/new")
    await page.getByTestId("room-name").fill("   ")
    await page.getByTestId("make-room").click()
    await expect(page).toHaveURL(/\/rooms\/new\?refused=/)
    await expect(page.getByRole("alert")).not.toBeEmpty()
  })
})
