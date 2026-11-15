import { expect, test as setup } from "@playwright/test"

import { signIn } from "./sign-in.ts"
import { admin, runner, stateOf } from "./values.mjs"

for (const person of [admin, runner]) {
  setup(`${person.email} signs in`, async ({ page }) => {
    await page.goto("/login")
    await signIn(page, person.email)
    await expect(page).toHaveURL(/\/chat$/)
    await page.context().storageState({ path: stateOf(person) })
  })
}
