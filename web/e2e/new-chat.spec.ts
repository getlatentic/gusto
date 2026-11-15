import { expect, sideRow, test, unique } from "./fixtures.ts"

test("a new chat starts from the box with a preset and a folder, and lands in its conversation in the side list", async ({
  page,
  gusto,
}) => {
  const preset = unique("Policy writer ")
  await gusto.savePreset(preset)
  const project = unique("project-")

  await page.goto("/chat")
  await page.getByTestId("message-box").fill("Draft a spending policy")

  await page.getByTestId("run-on").click()
  await page.getByTestId("choose-preset").click()
  await page.getByTestId("choice-option").filter({ hasText: preset }).click()
  await expect(page.getByTestId("choose-preset")).toContainText(preset)

  await page.getByTestId("folder-chip").click()
  const dialog = page.getByTestId("folder-dialog")
  await expect(dialog).toBeVisible()
  await dialog.getByTestId("new-folder").click()
  await dialog.getByTestId("new-folder-name").fill(project)
  await dialog.getByTestId("new-folder-name").press("Enter")
  await dialog.getByTestId("choose-folder").click()
  await expect(dialog).toBeHidden()
  await expect(page.getByTestId("folder-chip")).toContainText(project)

  await page.getByTestId("send").click()
  await expect(page).toHaveURL(/\/instances\/[^/]+\/threads\/[^/#?]+/)
  await expect(page.getByTestId("turn").getByTestId("said")).toHaveText(
    "Draft a spending policy"
  )
  const here = new URL(page.url()).pathname
  const group = page.getByTestId("side-project").filter({ hasText: project })
  await expect(sideRow(group, here)).toBeVisible()
})
