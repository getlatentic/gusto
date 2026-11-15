import { expect, sideRow, test, unique } from "./fixtures.ts"
import { signIn } from "./sign-in.ts"
import { admin, gustoURL, sharedPassword } from "./values.mjs"

test.describe("signing in", () => {
  test.use({ storageState: { cookies: [], origins: [] } })

  test("a person signed out who opens a conversation is sent to sign in and lands back on it", async ({
    page,
    gusto,
  }) => {
    const room = await gusto.makeRoom(unique("Signing in "))
    await page.goto(room.path)
    await signIn(page, admin.email)
    await expect(page).toHaveURL(gustoURL + room.path)
    await expect(page.getByTestId("conversation-title")).toHaveText(
      /^Signing in /
    )
  })

  test("a person whose session ends while the app is open signs in again and lands on the conversation they opened", async ({
    page,
    gusto,
  }) => {
    const first = await gusto.makeRoom(unique("Before "))
    const second = await gusto.makeRoom(unique("After "))
    await page.goto(first.path)
    await signIn(page, admin.email)
    await expect(page.getByTestId("conversation-title")).toHaveText(/^Before /)
    // The session ends. The room's socket drops and cannot come back, so the app asks whether the person is still signed in,
    // which is refused outright: a redirect to sign in would be remembered, and signing in would land on the version.
    await page.context().clearCookies()
    const refused = page.waitForResponse(
      (answer) => answer.url().endsWith(`${first.path}/version`),
      { timeout: 30_000 }
    )
    await page.context().setOffline(true)
    await page.context().setOffline(false)
    expect((await refused).status()).toBe(401)
    // The app's next read of a page's data is refused too, and it loads the page, which asks to sign in.
    await sideRow(page.getByTestId("side-rooms"), second.path).click()
    await signIn(page, admin.email)
    await expect(page).toHaveURL(gustoURL + second.path)
    await expect(page.getByTestId("conversation-title")).toHaveText(/^After /)
  })

  test("the shared password signs a person in and lands them on the conversation they opened", async ({
    page,
    gusto,
  }) => {
    const room = await gusto.makeRoom(unique("Shared "))
    await page.goto(room.path)
    await expect(page).toHaveURL(/\/login/)
    await page.getByTestId("use-shared-password").click()
    await page.getByTestId("shared-password").fill(sharedPassword)
    await page.getByTestId("sign-in").click()
    await expect(page).toHaveURL(gustoURL + room.path)
    await expect(page.getByTestId("conversation-title")).toHaveText(/^Shared /)
  })
})
