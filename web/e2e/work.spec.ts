import { expect, test, type Gusto } from "./fixtures.ts"

// startAgent starts one of the person's own agents, the canned one, as the page that sets one up does, and gives its first
// conversation and the project it belongs to.
async function startAgent(gusto: Gusto) {
  const landed = await gusto.post("/instances", {
    harness: "opencode",
    model: "stub/stub-model",
  })
  const thread = landed.split(/[?#]/)[0]
  return { thread, project: thread.split("/threads/")[0] }
}

test.describe("work", () => {
  test("a conversation's task appears on Activity and opens its page, which leads to its reply and file", async ({
    page,
    gusto,
  }) => {
    const { thread } = await startAgent(gusto)
    const turn = await gusto.say(thread, "Write the spending policy")
    await gusto.replied(thread, turn)

    await page.goto("/jobs")
    const run = page.locator(`[data-testid="run"][data-id="${turn}"]`)
    await expect(run).toBeVisible()
    await expect(run.getByTestId("run-brief")).toHaveText(
      "Write the spending policy"
    )
    await run.getByTestId("run-brief").click()

    await expect(page).toHaveURL(new RegExp(`/jobs/${turn}$`))
    await expect(page.getByTestId("task-brief")).toHaveText(
      "Write the spending policy"
    )
    await expect(page.getByText("Message 1 of 1")).toBeVisible()
    await expect(page.getByTestId("task-state")).toHaveText("replied")
    const proof = page.getByTestId("task-proof")
    await proof.getByRole("button", { name: "Verify this run" }).click()
    await proof
      .getByRole("button", { name: "Each file and its fingerprint" })
      .click()
    await expect(
      proof.getByRole("link", { name: "transcript.log" })
    ).toBeVisible()

    await page.getByRole("link", { name: "Open the conversation" }).click()
    await expect(page).toHaveURL(new RegExp(`${thread}$`))
    await expect(page.getByText("stub says hi").first()).toBeVisible()
    await expect(
      page.getByTestId("file-chip").filter({ hasText: "policy.md" }).first()
    ).toBeVisible()
  })

  test("a project's page lists its conversations and changes its model", async ({
    page,
    gusto,
  }) => {
    const { thread, project } = await startAgent(gusto)
    const turn = await gusto.say(thread, "What did we spend?")
    await gusto.replied(thread, turn)

    await page.goto(project)
    const conversation = page
      .getByTestId("conversation")
      .and(page.locator(`[href="${thread}"]`))
    await expect(conversation).toContainText("What did we spend?")

    const change = page.getByTestId("change-model")
    await page.getByTestId("model").click()
    await page.getByRole("option", { name: /Stub Large/ }).click()
    await change.getByRole("button", { name: "Use" }).click()

    await expect
      .poll(async () => {
        const answer = await page.request.get(`/app/api${project}`)
        return ((await answer.json()) as { Instance: { Model: string } })
          .Instance.Model
      })
      .toBe("stub/stub-large")
    await expect(page.getByTestId("model")).toContainText("Stub Large")
    await page.reload()
    await expect(page.getByTestId("model")).toContainText("Stub Large")

    await conversation.click()
    await expect(page).toHaveURL(new RegExp(`${thread}$`))
  })

  test("a running task can be stopped from its page", async ({
    page,
    gusto,
  }) => {
    const { project } = await startAgent(gusto)
    // The check takes long enough to stop the task while it runs.
    const landed = await gusto.post(`${project}/tasks`, {
      brief: "Count the files, slowly",
      gate: "sleep 30",
    })
    const task = landed.split(/[?#]/)[0]

    await page.goto(task)
    await expect(page.getByTestId("task-brief")).toHaveText(
      "Count the files, slowly"
    )
    await expect(page.getByTestId("task-stream")).toBeVisible()
    await page
      .getByTestId("stop-task")
      .getByRole("button", { name: "Stop" })
      .click()

    await expect(page.getByTestId("task-state")).toHaveText("interrupted")
    await expect(page.getByTestId("stop-task")).toHaveCount(0)
  })
})

// sideways is how far the page, or its main pane, reaches past the window's width, which a phone would scroll sideways for.
const sideways = (page: import("@playwright/test").Page) =>
  page.evaluate(() =>
    [
      document.scrollingElement ?? document.documentElement,
      document.getElementById("main"),
    ]
      .filter((each): each is Element => each !== null)
      .map((each) => each.scrollWidth - each.clientWidth)
      .reduce((most, each) => Math.max(most, each), 0)
  )

test.describe("the work pages at a phone's width", () => {
  test.use({ viewport: { width: 375, height: 812 }, isMobile: true })

  test("do not scroll sideways", async ({ page, gusto }) => {
    const { thread, project } = await startAgent(gusto)
    const turn = await gusto.say(thread, "Write the spending policy")
    await gusto.replied(thread, turn)
    for (const path of [
      "/jobs",
      `/jobs/${turn}`,
      project,
      "/compare",
      "/engines",
    ]) {
      await page.goto(path)
      await expect(page.getByTestId("page-title")).toBeVisible()
      await expect(page.locator('[aria-busy="true"]')).toHaveCount(0)
      expect(await sideways(page), path).toBe(0)
    }
  })
})
