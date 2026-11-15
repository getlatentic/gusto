import { randomInt } from "node:crypto"

import { expect, test, unique } from "./fixtures.ts"

// dollars is an amount as a figure of the console shows it: to the cent, with no grouping.
const dollars = (amount: number) => `$${amount.toFixed(2)}`

test.describe("the console", () => {
  test("an admin sets both budgets in the Budgets dialog and sees them", async ({
    page,
  }) => {
    const each = randomInt(10, 99) + 0.25
    const organization = randomInt(1000, 9999) + 0.5
    await page.goto("/admin/budgets")
    await page.getByTestId("change-budgets").click()

    const dialog = page.getByTestId("budgets-dialog")
    await expect(dialog).toBeVisible()
    await dialog.getByTestId("budget-each").fill(String(each))
    await dialog.getByTestId("budget-org").fill(String(organization))
    await dialog.getByTestId("save-budgets").click()

    await expect(dialog).toBeHidden()
    await expect(
      page.getByTestId("each-person-budget").getByTestId("figure-value")
    ).toHaveText(dollars(each))
    await expect(
      page.getByTestId("organization-budget").getByTestId("figure-value")
    ).toHaveText(dollars(organization))
  })

  test("an admin makes an API token and sees its secret once", async ({
    page,
  }) => {
    const name = unique("ci-")
    await page.goto("/admin/tokens")
    // The form is open already while no token has been made; otherwise New token opens it.
    const newToken = page.getByTestId("new-token")
    const field = page.getByTestId("token-name")
    await expect(newToken.or(field)).toBeVisible()
    if (await newToken.isVisible()) await newToken.click()
    await field.fill(name)
    await page.getByTestId("make-token").click()

    await expect(page.getByTestId("token-secret")).toHaveText(/^\S{20,}$/)
    const row = page.getByTestId("token-row").filter({ hasText: name })
    await expect(row).toBeVisible()

    await page.reload()
    await expect(row).toBeVisible()
    await expect(page.getByTestId("token-secret")).toHaveCount(0)
  })
})
