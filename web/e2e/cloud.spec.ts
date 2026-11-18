import { expect, test, type Page } from "@playwright/test"

import { codeIn, linkIn, mailTo } from "./cloud-mail.ts"
import { bookURL, cloudURL, operatorURL } from "./values.mjs"

// gusto cloud is its own site, so each test starts with nobody signed in to it. The operator's test pauses demos for every
// visitor, so this file's tests run one at a time.
test.use({ storageState: { cookies: [], origins: [] } })
test.describe.configure({ mode: "serial" })

// A company of its own for each test, so a sign-up is never refused for an address or a domain another test used.
const company = (test: string) =>
  `${test}-${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`

async function signUp(page: Page, email: string) {
  await page.goto(`${cloudURL}/signup`)
  await page.getByTestId("email").fill(email)
  await page.getByTestId("sign-up").click()
  await expect(page.getByRole("status")).toBeVisible()
}

test.describe("gusto cloud", () => {
  test("the landing page says what gusto is and leads to sign-up and a call", async ({
    page,
  }) => {
    await page.goto(cloudURL)
    await expect(
      page.getByRole("heading", {
        level: 1,
        name: "Bring your team and AI agents together.",
      })
    ).toBeVisible()
    await expect(page).toHaveTitle(/gusto/)
    await expect(page.locator('meta[name="robots"]')).toHaveAttribute(
      "content",
      /^index/
    )
    await expect(page.getByTestId("start").first()).toHaveAttribute(
      "href",
      "/signup"
    )
    await expect(page.getByTestId("book").first()).toHaveAttribute(
      "href",
      bookURL
    )
    await expect(page.getByTestId("agents").getByRole("listitem")).toHaveCount(
      6
    )
    await page.getByTestId("sign-in").click()
    await expect(page).toHaveURL(`${cloudURL}/signin`)
  })

  test("a refused sign-up says why and keeps what was typed", async ({
    page,
  }) => {
    await page.goto(`${cloudURL}/signup`)
    await page.getByTestId("email").fill("someone@gmail.com")
    await page.getByTestId("sign-up").click()
    await expect(page.getByTestId("refusal")).toBeVisible()
    await expect(page.getByTestId("email")).toHaveValue("someone@gmail.com")
  })

  test("signing up makes the organization, and its confirmation link verifies the address once pressed", async ({
    page,
  }) => {
    const email = `ada@${company("signup")}.com`
    await signUp(page, email)
    await page.goto(`${cloudURL}/org`)
    await expect(page.getByTestId("email-state")).toHaveText("Not verified")
    await expect(
      page.getByTestId("figure-people").getByTestId("figure-value")
    ).toHaveText(/^1 of /)

    const link = linkIn(await mailTo(email, /Confirm your email address/))
    await page.goto(link)
    await expect(
      page.getByRole("heading", { name: "Confirm your email address" })
    ).toBeVisible()
    await page.getByTestId("confirm").click()
    await page.goto(`${cloudURL}/org`)
    await expect(page.getByTestId("email-state")).toHaveText("Verified")
  })

  test("a person signs in with the code mailed to them", async ({ page }) => {
    const email = `grace@${company("signin")}.com`
    await signUp(page, email)
    await page.goto(`${cloudURL}/org`)
    await page.getByTestId("sign-out").click()

    await page.goto(`${cloudURL}/signin`)
    await page.getByTestId("email").fill(email)
    await page.getByTestId("send-code").click()
    await expect(
      page.getByRole("heading", { name: "Check your email" })
    ).toBeVisible()
    await page.getByTestId("code").fill("000000")
    await page.getByTestId("sign-in").click()
    await expect(page.getByTestId("refusal")).toBeVisible()

    const code = codeIn(await mailTo(email, /Your gusto sign-in code/))
    await page.getByTestId("code").fill(code)
    await page.getByTestId("sign-in").click()
    await page.goto(`${cloudURL}/org`)
    await expect(page.getByTestId("org-name")).toBeVisible()
  })

  test("the demo form keeps what was typed when it is refused", async ({
    page,
  }) => {
    await page.goto(`${cloudURL}/demo`)
    await page.getByTestId("demo-name").fill("Ada")
    await page.getByTestId("demo-email").fill("ada@gmail.com")
    await page.getByTestId("demo-company").fill("Acme")
    await page.getByTestId("start-demo").click()
    await expect(page.getByTestId("refusal")).toBeVisible()
    await expect(page.getByTestId("demo-email")).toHaveValue("ada@gmail.com")
    await expect(page.getByTestId("demo-company")).toHaveValue("Acme")
  })
})

// gusto cloud serves each organization at /workspace/<slug> on its own name. Going in lands there with gusto cloud's sign-in
// and no other, the app reads its data there and asks gusto cloud about the person, and its links, its forms and a reload
// all stay there.
test("an organization opens at its own path, and its pages stay under it", async ({
  page,
}) => {
  test.setTimeout(120_000)
  const failed: string[] = []
  const built: string[] = []
  page.on("response", (answer) => {
    const asked = answer.url()
    if (asked.includes("/app/assets/")) built.push(new URL(asked).pathname)
    if (
      (asked.includes("/app/api/") ||
        asked.includes(".json?org=") ||
        asked.includes("/app/assets/")) &&
      answer.status() >= 400
    )
      failed.push(`${answer.status()} ${asked}`)
  })
  await signUp(page, `ada@${company("paths")}.com`)
  await page.goto(`${cloudURL}/open`)
  const under = new RegExp(`^${cloudURL}/workspace/[a-z0-9-]+/`)
  await expect(page).toHaveURL(under, { timeout: 90_000 })
  const workspace = new URL(page.url()).pathname
    .split("/")
    .slice(0, 3)
    .join("/")
  await expect(page.getByRole("navigation").first()).toBeVisible()
  const signIns = (await page.context().cookies())
    .map((cookie) => cookie.name)
    .filter((name) => /gusto(_cloud)?$/.test(name))
  expect(signIns).toEqual(["gusto_cloud"])

  const files = page.locator('a[href$="/files"]').first()
  await expect(files).toHaveAttribute("href", `${workspace}/files`)
  await files.click()
  await expect(page).toHaveURL(`${cloudURL}${workspace}/files`)
  await page.reload()
  await expect(page).toHaveURL(`${cloudURL}${workspace}/files`)
  await expect(page.getByRole("navigation").first()).toBeVisible()
  expect(failed).toEqual([])
  // The page's own build comes from its own server, under its path, so a deploy cannot leave it another release's files.
  const product = built.filter((path) => !path.startsWith("/app/"))
  expect(product.length).toBeGreaterThan(0)
  expect(
    product.every((path) => path.startsWith(`${workspace}/app/assets/`))
  ).toBe(true)
})

test.describe("the operator's page", () => {
  test("pauses new demos and takes them again", async ({ page }) => {
    await page.goto(operatorURL)
    await expect(
      page.getByRole("heading", { level: 1, name: "Demos" })
    ).toBeVisible()
    await page.getByTestId("pause").click()
    await expect(page.getByTestId("notice")).toHaveText(/New demos are paused/)
    await expect(page.getByTestId("pause")).toHaveText("Accept new demos")
    await page.getByTestId("pause").click()
    await expect(page.getByTestId("pause")).toHaveText("Pause new demos")
  })
})
