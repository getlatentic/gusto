import { expect, roomWithAgent, test } from "./fixtures.ts"

test("two people in a room see each other, see each other typing, and see an agent's reply arrive without reloading", async ({
  page,
  runnerPage,
  gusto,
}) => {
  const { room, agent } = await roomWithAgent(gusto)
  await page.goto(room.path)
  await runnerPage.goto(room.path)

  await expect(page.getByTestId("here-person")).toHaveAttribute(
    "title",
    "Grace is here"
  )
  await expect(runnerPage.getByTestId("here-person")).toHaveAttribute(
    "title",
    "Ada is here"
  )

  const box = runnerPage.getByTestId("message-box")
  await box.pressSequentially(`@${agent.slice(0, 5)}`)
  const menu = runnerPage.getByTestId("mention-menu")
  await expect(menu).toBeVisible()
  await expect(page.getByTestId("typing")).toHaveText(/^Grace is typing/)
  await menu.getByTestId("mention-option").filter({ hasText: agent }).click()
  await expect(menu).toBeHidden()
  await box.pressSequentially("draft a spending policy")
  await expect(box).toHaveValue(`@${agent} draft a spending policy`)
  await runnerPage.getByTestId("send").click()

  const turn = page.getByTestId("turn").filter({
    has: page
      .getByTestId("said")
      .filter({ hasText: "draft a spending policy" }),
  })
  await expect(turn.getByTestId("said")).toHaveText(
    `@${agent} draft a spending policy`
  )
  await expect(turn.getByTestId("reply")).toHaveAttribute("data-done", "")
  await expect(turn.getByTestId("reply")).toContainText("stub says hi")
})
