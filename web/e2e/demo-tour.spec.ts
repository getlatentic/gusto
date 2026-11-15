import { expect, test } from "./fixtures.ts"

// Behind gusto cloud a demo host adds the lease bar and the tour to every page, and they draw with the app's stylesheet. This
// server is not behind one, so the test adds both scripts and answers the lease as a demo host would.
test("on a demo host the lease bar and the tour draw on the app's pages", async ({
  page,
}) => {
  await page.route("**/.gusto/lease", (route) =>
    route.fulfill({
      json: { seconds: 1200, demo: true, book: "https://cal.example/demo" },
    })
  )
  await page.goto("/chat")
  await expect(page.getByTestId("message-box")).toBeVisible()
  await page.addScriptTag({ url: "/static/guide.js" })
  await page.addScriptTag({ url: "/static/lease.js" })

  const bar = page.getByTestId("lease-bar")
  await expect(bar).toContainText("Demo ends in 20:00")
  await expect(bar).toHaveCSS("position", "fixed")
  const ink = await page.evaluate(() =>
    getComputedStyle(document.documentElement).getPropertyValue("--gu-text")
  )
  expect(ink.trim()).not.toBe("")
  await expect(bar).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)")

  const tour = page.getByTestId("demo-tour")
  await expect(tour).toBeVisible()
  await expect(tour).toHaveCSS("position", "fixed")
  await expect(tour).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)")
  await expect(tour).not.toHaveCSS("border-top-left-radius", "0px")
  await expect(tour.getByRole("listitem")).toHaveCount(4)
  await expect(tour).toContainText("0 of 4")
  // The tour sits above the lease bar, and beside the rail, whose foot is who is signed in.
  const [tourBox, barBox] = [await tour.boundingBox(), await bar.boundingBox()]
  expect(tourBox!.y + tourBox!.height).toBeLessThanOrEqual(barBox!.y)
  const account = await page
    .getByRole("button", { name: /account and settings/ })
    .boundingBox()
  expect(tourBox!.x).toBeGreaterThanOrEqual(account!.x + account!.width)
})
