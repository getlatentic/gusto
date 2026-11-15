import { fireEvent, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { ActivityPage } from "@/pages/work/activity-page"
import { activityOf, runOf } from "@/pages/work/fixture"
import type { ActivityData, Index } from "@/pages/work/types"
import { renderPage } from "@/test/render"

function draw(page: ActivityData, query = "") {
  const at = `/jobs${query}`
  return renderPage(at, "/jobs", <ActivityPage />, { [at]: page })
}

const index = (change: Partial<Index> = {}): Index => ({
  Query: "",
  Person: "",
  Everyone: true,
  Sort: "",
  Sorts: [
    { Name: "queued", Label: "Newest asked" },
    { Name: "ended", Label: "Recently ended" },
    { Name: "seq", Label: "Order of arrival" },
  ],
  Building: { building: false, done: 0, total: 0 },
  Newest: "",
  ...change,
})

describe("Activity", () => {
  it("lists each run under its day, with where it came from, its model, when and what it cost", async () => {
    draw(
      activityOf({
        Days: [
          {
            Named: "Today",
            Runs: [
              runOf(),
              runOf({
                ID: "t2",
                Brief: "say hello",
                State: "failed",
                Outcome: "failed",
                On: {
                  Named: "API",
                  Project: "",
                  Why: "none: sent to POST /api/jobs",
                  Link: "",
                },
                Brain: "",
                Cost: {
                  Text: "1.2k tokens",
                  Title: "3 calls, and no price for this model",
                },
              }),
            ],
          },
        ],
      })
    )
    const [first, second] = await screen.findAllByTestId("run")
    expect(within(first).getByTestId("run-brief").getAttribute("href")).toBe(
      "/jobs/t1"
    )
    const origin = within(first).getByRole("link", { name: "Atlas" })
    expect(origin.getAttribute("href")).toBe("/instances/atlas-1")
    expect(origin.getAttribute("title")).toBe("Atlas · Counting")
    expect(first.textContent).toContain("Atlas · big-1")
    expect(first.textContent).toContain("10:00 · $0.0100")
    expect(within(first).queryByText("check passed")).toBeNull()
    expect(within(second).getByText("API").getAttribute("title")).toBe(
      "none: sent to POST /api/jobs"
    )
    expect(within(second).getByText("failed").getAttribute("title")).toBe(
      "15 Nov 10:00"
    )
    expect(within(second).getByText("1.2k tokens").getAttribute("title")).toBe(
      "3 calls, and no price for this model"
    )
    expect(screen.getByRole("heading", { name: "Today" })).toBeTruthy()
    expect(
      screen.getByText("Every task agents ran, newest first · 3 tasks")
    ).toBeTruthy()
  })

  it("says who asked for a run that waits, and why it waits", async () => {
    draw(
      activityOf({
        Days: [
          {
            Named: "Today",
            Runs: [
              runOf({
                State: "queued",
                Outcome: "waiting",
                AskedBy: "amy@acme.test",
                Waiting:
                  "Waiting for this agent to finish its last message. Number 1 in line.",
              }),
            ],
          },
        ],
      })
    )
    expect(
      await screen.findByText(
        "Asked by amy@acme.test. Waiting for this agent to finish its last message. Number 1 in line."
      )
    ).toBeTruthy()
    expect(screen.getByText("waiting")).toBeTruthy()
  })

  it("counts what happened as tabs that filter, leaving out a state with none", async () => {
    draw(activityOf({ Harness: "codex" }))
    const tabs = await screen.findByRole("tablist", {
      name: "What happened, this week",
    })
    const names = within(tabs)
      .getAllByRole("tab")
      .map((tab) => tab.textContent)
    expect(names).toEqual(["Everything", "passed 2", "failed 1"])
    expect(
      within(tabs).getByRole("tab", { name: "Everything" }).getAttribute("href")
    ).toBe("/jobs?agent_type=codex")
    expect(
      within(tabs).getByRole("tab", { name: "failed 1" }).getAttribute("href")
    ).toBe("/jobs?state=failed")
  })

  it("offers a search, a person to an admin, and an order, when it reads the index", async () => {
    draw(activityOf({ State: "failed", Index: index({ Query: "login" }) }))
    const filters = await screen.findByTestId("activity-filters")
    const field = (name: string) =>
      filters.querySelector(`[name="${name}"]`) as HTMLInputElement
    expect(field("state").value).toBe("failed")
    expect(field("q").value).toBe("login")
    expect(field("person")).toBeTruthy()
    expect(screen.getByTestId("sort").textContent).toContain("Newest asked")
    expect(screen.queryByTestId("agent-type")).toBeNull()
  })

  it("does not offer a choice of people to a person who sees only their own", async () => {
    draw(
      activityOf({
        Index: index({ Everyone: false }),
        Harnesses: ["codex", "pi"],
      })
    )
    const filters = await screen.findByTestId("activity-filters")
    expect(filters.querySelector('[name="person"]')).toBeNull()
    expect(screen.getByTestId("agent-type").textContent).toContain(
      "Every agent"
    )
  })

  it("searches by the address the form makes", async () => {
    const { asked } = renderPage("/jobs", "/jobs", <ActivityPage />, {
      "/jobs": activityOf({ Index: index() }),
      "/jobs?q=adder&person=&sort=queued": activityOf({
        Index: index({ Query: "adder" }),
        Days: [],
      }),
    })
    const search = (await screen.findByPlaceholderText(
      "Search tasks"
    )) as HTMLInputElement
    fireEvent.change(search, { target: { value: "adder" } })
    fireEvent.submit(search.closest("form")!)
    expect(
      await screen.findByText("Nothing matches these filters.")
    ).toBeTruthy()
    expect(asked).toContain("/app/api/jobs?q=adder&person=&sort=queued")
  })

  it("says the index is still being built, and what that leaves out", async () => {
    draw(
      activityOf({
        Days: [],
        Index: index({ Building: { building: true, done: 3, total: 12 } }),
      })
    )
    expect(
      await screen.findByText(
        "The index is still being built, 3 of 12 tasks. What is below is not yet everything."
      )
    ).toBeTruthy()
    expect(screen.getByTestId("activity-empty").textContent).toBe(
      "Nothing is listed yet: the index is still being built."
    )
  })

  it("says where to start when nothing has happened", async () => {
    draw(activityOf({ Days: [], Figures: [], Harnesses: [] }))
    const empty = await screen.findByTestId("activity-empty")
    expect(empty.textContent).toBe(
      "Nothing has happened here yet. Everything an agent does shows up here: start one."
    )
    expect(
      within(empty)
        .getByRole("link", { name: "start one" })
        .getAttribute("href")
    ).toBe("/presets")
    expect(screen.queryByTestId("activity-filters")).toBeNull()
  })

  it("leads to the newest, newer and older pages", async () => {
    draw(
      activityOf({
        Index: index({ Newest: "/jobs" }),
        Older: "/jobs?after=abc",
      })
    )
    const pages = await screen.findByRole("navigation", { name: "Pages" })
    expect(
      within(pages)
        .getAllByRole("link")
        .map((link) => [link.textContent, link.getAttribute("href")])
    ).toEqual([
      ["Newest", "/jobs"],
      ["Older", "/jobs?after=abc"],
    ])
  })
})
