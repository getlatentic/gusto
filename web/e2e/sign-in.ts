import { expect, type Page } from "@playwright/test"

// signIn signs a person in as they would on a server with an identity provider: the sign-in page, the provider's page, and back.
// It starts wherever the browser was sent to sign in.
export async function signIn(page: Page, email: string) {
  await expect(page).toHaveURL(/\/login/)
  await page.getByTestId("provider-sign-in").click()
  await page.getByTestId("provider-email").fill(email)
  await page.getByTestId("provider-continue").click()
}
