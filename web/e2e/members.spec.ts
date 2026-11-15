import { expect, test, unique } from "./fixtures.ts"

// row is a member's row in the list, found by where it leads.
const row = (page: import("@playwright/test").Page, where: string) =>
  page
    .getByTestId("members-list")
    .locator(`[data-testid="member-row"][data-href="${where}"]`)

test.describe("members", () => {
  test("a new agent is made in the dialog, opens on its page, and is listed", async ({
    page,
  }) => {
    const name = unique("Scout")
    await page.goto("/members?new=1")
    const dialog = page.getByTestId("new-agent-dialog")
    await expect(dialog).toBeVisible()
    await dialog.getByLabel("Name").fill(name)
    await dialog
      .getByTestId("new-agent-form")
      .locator('button[type="submit"]')
      .click()

    await expect(page).toHaveURL(/\/members\/agents\/[^/?]+/)
    await expect(page.getByTestId("agent-page")).toBeVisible()
    const where = new URL(page.url()).pathname
    await expect(row(page, where)).toBeVisible()
  })

  test("a name already taken is refused, and the dialog comes back with why", async ({
    page,
    gusto,
  }) => {
    const name = unique("Twin")
    await gusto.makeAgent(name)
    await page.goto("/members?new=1")
    const dialog = page.getByTestId("new-agent-dialog")
    await dialog.getByLabel("Name").fill(name)
    await dialog
      .getByTestId("new-agent-form")
      .locator('button[type="submit"]')
      .click()

    await expect(page).toHaveURL(/\/members\?new=1&refused=/)
    await expect(
      page.getByTestId("new-agent-dialog").getByRole("alert")
    ).toContainText(name)
  })

  test("an agent's persona is saved, and is there after a reload", async ({
    page,
    gusto,
  }) => {
    const id = await gusto.makeAgent(unique("Writer"))
    await page.goto(`/members/agents/${id}?tab=profile`)
    const persona = "You answer in short sentences."
    await page.getByTestId("persona").fill(persona)
    await page.getByTestId("save-profile").click()
    await expect(page.getByTestId("persona")).toHaveValue(persona)
    await page.reload()
    await expect(page.getByTestId("persona")).toHaveValue(persona)
  })

  test("messaging an agent opens its own room, which is in the side list", async ({
    page,
    gusto,
  }) => {
    const id = await gusto.makeAgent(unique("Direct"))
    await page.goto(`/members/agents/${id}`)
    await page
      .getByTestId("message-agent-form")
      .locator('button[type="submit"]')
      .click()
    await expect(page).toHaveURL(/\/instances\/[^/]+\/threads\/[^/]+$/)
    const path = new URL(page.url()).pathname
    await expect(
      page
        .getByTestId("side-list")
        .locator(`[data-testid="side-row"][href="${path}"]`)
    ).toBeVisible()
  })

  test("removing an agent asks first, then leaves the list without it", async ({
    page,
    gusto,
  }) => {
    const id = await gusto.makeAgent(unique("Spare"))
    const where = `/members/agents/${id}`
    await page.goto(where)
    await expect(row(page, where)).toBeVisible()
    await page
      .getByTestId("remove-agent-form")
      .locator('button[type="submit"]')
      .click()
    await page.getByTestId("confirm-remove").click()
    await expect(page).toHaveURL(/\/members$/)
    await expect(row(page, where)).toHaveCount(0)
  })

  test("a runner sees an agent of someone else's, with nothing to change in it", async ({
    gusto,
    runnerPage,
  }) => {
    const id = await gusto.makeAgent(unique("Shared"))
    await runnerPage.goto(`/members/agents/${id}`)
    await expect(runnerPage.getByTestId("agent-page")).toBeVisible()
    await expect(runnerPage.getByTestId("persona")).toHaveCount(0)
    await expect(runnerPage.getByTestId("remove-agent-form")).toHaveCount(0)
  })
})
