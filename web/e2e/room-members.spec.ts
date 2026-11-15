import { expect, test, unique } from "./fixtures.ts"
import { runner } from "./values.mjs"

test.describe("a room's members", () => {
  test("the address with ?members=1 opens the members dialog, where an admin changes the runner's role", async ({
    page,
    gusto,
  }) => {
    const room = await gusto.makeRoom(unique("Members "))
    await gusto.addToRoom(room, { people: [runner.email] })
    await page.goto(`${room.path}?members=1`)

    const dialog = page.getByTestId("members-dialog")
    await expect(dialog).toBeVisible()
    const role = dialog
      .getByTestId("room-person")
      .and(page.locator(`[data-who="${runner.email}"]`))
      .getByTestId("room-role")
    await expect(role).toHaveText(/^Member/)
    await role.click()
    await page
      .getByTestId("choice-option")
      .and(page.locator('[data-value="admin"]'))
      .click()
    await expect(role).toHaveText(/^Admin/)

    await page.reload()
    await expect(role).toHaveText(/^Admin/)
  })

  test("adding a bad address with no agents chosen shows the refusal in the add dialog", async ({
    page,
    gusto,
  }) => {
    const room = await gusto.makeRoom(unique("Adding "))
    await page.goto(`${room.path}?members=1`)
    await page.getByTestId("members-dialog").getByTestId("add-member").click()

    const dialog = page.getByTestId("add-dialog")
    await expect(dialog).toBeVisible()
    await dialog.getByTestId("add-people").fill("not an address")
    await dialog.getByTestId("add-submit").click()
    await expect(dialog.getByTestId("add-refused")).toHaveText(
      /sign-in address/
    )
    await expect(page).toHaveURL(/[?&]add=1/)
  })
})
