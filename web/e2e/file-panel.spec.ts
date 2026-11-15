import { expect, roomWithAgent, test, type Gusto } from "./fixtures.ts"

// roomWithFile is a room whose agent has replied with policy.md.
async function roomWithFile(gusto: Gusto) {
  const { room, agent } = await roomWithAgent(gusto)
  const turn = await gusto.say(room.path, `@${agent} write the policy`)
  await gusto.replied(room.path, turn)
  return room
}

test.describe("a reply's file", () => {
  test.describe("on a wide window", () => {
    test.use({ viewport: { width: 1440, height: 900 } })

    test("opens beside the conversation, in a frame of the file's page", async ({
      page,
      gusto,
    }) => {
      const room = await roomWithFile(gusto)
      await page.goto(room.path)
      await page.getByTestId("file-chip").click()

      const panel = page.getByTestId("file-panel")
      await expect(panel).toBeVisible()
      const frame = panel.getByTestId("file-frame")
      await expect(frame).toHaveAttribute(
        "src",
        /\/file\?path=policy\.md&.*embed=1/
      )
      await expect(frame.contentFrame().locator("body")).toContainText(
        "Spending policy"
      )
      await expect(page).toHaveURL(new RegExp(`${room.path}$`))
    })
  })

  test.describe("on a narrow window", () => {
    test.use({ viewport: { width: 900, height: 900 } })

    test("opens as a page of its own", async ({ page, gusto }) => {
      const room = await roomWithFile(gusto)
      await page.goto(room.path)
      await page.getByTestId("file-chip").click()

      await expect(page).toHaveURL(
        new RegExp(`/instances/${room.id}/file\\?path=policy\\.md`)
      )
      await expect(page.getByTestId("file-panel")).toHaveCount(0)
      await expect(page.locator("body")).toContainText("Spending policy")
    })
  })
})
