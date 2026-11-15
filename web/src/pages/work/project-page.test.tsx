import { fireEvent, screen, within } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { projectOf } from "@/pages/work/fixture"
import { ProjectPage } from "@/pages/work/project-page"
import type { ProjectData } from "@/pages/work/types"
import { renderPage } from "@/test/render"

function draw(page: ProjectData) {
  return renderPage("/instances/atlas-1", "/instances/:id", <ProjectPage />, {
    "/instances/atlas-1": page,
  })
}

const field = (scope: Element, name: string) =>
  scope.querySelector(`[name="${name}"]`) as HTMLInputElement | null

describe("a project's page", () => {
  it("names the agent and its model, and lists its conversations newest first", async () => {
    draw(
      projectOf({
        Instance: { ...projectOf().Instance, Sensitivity: "restricted" },
      })
    )
    expect(
      (await screen.findByText("stays in your network")).parentElement!
        .textContent
    ).toBe("big-1 · stays in your network")
    const rows = within(screen.getByTestId("conversations")).getAllByTestId(
      "conversation"
    )
    expect(
      rows.map((row) => [row.getAttribute("href"), row.textContent])
    ).toEqual([
      ["/instances/atlas-1/threads/2", "The budgetwhat did we spend?"],
      ["/instances/atlas-1/threads/1", "New threadNothing said yet"],
    ])
    expect(screen.getByTestId("page-title").textContent).toBe("Atlas")
    expect(
      screen
        .getAllByRole("link", { name: "Files" })
        .map((link) => link.getAttribute("href"))
    ).toEqual(["/files", "/instances/atlas-1/files"])
    expect(screen.getByTestId("new-conversation").getAttribute("action")).toBe(
      "/instances/atlas-1/threads"
    )
  })

  it("says why a form was refused", async () => {
    draw(projectOf({ Refused: "choose a model" }))
    expect((await screen.findByRole("alert")).textContent).toBe(
      "choose a model"
    )
  })

  it("changes the model among more than one, and only names one", async () => {
    const { unmount } = draw(projectOf())
    const change = await screen.findByTestId("change-model")
    expect(change.getAttribute("action")).toBe("/instances/atlas-1/model")
    expect(field(change, "model")!.value).toBe("acme/big-1")
    expect(screen.getByTestId("model").textContent).toContain(
      "big-1 · $1.00 in, $2.00 out per million tokens"
    )
    unmount()
    draw(projectOf({ Models: [{ value: "acme/big-1", label: "big-1" }] }))
    const about = (await screen.findByText("Model")).closest("dl")!
    expect(screen.queryByTestId("change-model")).toBeNull()
    expect(about.textContent).toContain(
      "Modelbig-1Agent typeCodexStarted15 Nov 09:00"
    )
  })

  it("names its skills and repository when it has them", async () => {
    draw(
      projectOf({
        Instance: {
          ...projectOf().Instance,
          Setup: "engineering",
          Repo: "https://example.com/r.git",
        },
      })
    )
    expect(
      (await screen.findByRole("link", { name: "engineering" })).getAttribute(
        "href"
      )
    ).toBe("/setups/engineering")
    expect(screen.getByText("https://example.com/r.git")).toBeTruthy()
  })

  it("lists its routines with when they run and how they last went, and offers to add one", async () => {
    draw(
      projectOf({
        Routines: [
          {
            ID: "r1",
            Title: "Morning",
            Prompt: "what changed?",
            Thread: "3",
            When: "every 6 hours",
            Last: "ran",
            Paused: false,
          },
          {
            ID: "r2",
            Title: "Weekly",
            Prompt: "sum up",
            Thread: "4",
            When: "daily at 08:00",
            Last: "skipped: still answering",
            Paused: true,
          },
        ],
        Unread: "routines.json: unexpected end of JSON input",
      })
    )
    const routines = await screen.findByTestId("routines")
    expect(within(routines).getByRole("alert").textContent).toBe(
      "No routine runs until this is mended: routines.json: unexpected end of JSON input"
    )
    const [morning, weekly] = within(routines).getAllByTestId("routine")
    const link = within(morning).getByRole("link", { name: "Morning" })
    expect(link.getAttribute("href")).toBe("/instances/atlas-1/threads/3")
    expect(link.getAttribute("title")).toBe("what changed?")
    expect(morning.textContent).toContain("every 6 hoursLast run ok")
    expect(
      within(morning)
        .getByRole("button", { name: "Pause" })
        .closest("form")!
        .getAttribute("action")
    ).toBe("/instances/atlas-1/routines/r1/pause")
    expect(weekly.textContent).toContain(
      "daily at 08:00PausedLast run skipped: still answering"
    )
    expect(
      within(weekly)
        .getByRole("button", { name: "Resume" })
        .closest("form")!
        .getAttribute("action")
    ).toBe("/instances/atlas-1/routines/r2/resume")
    fireEvent.click(
      within(routines).getByRole("button", { name: "Add a routine" })
    )
    const add = within(routines)
      .getByRole("button", { name: "Add it" })
      .closest("form")!
    expect(add.getAttribute("action")).toBe("/instances/atlas-1/routines")
    expect(
      ["title", "daily", "every", "prompt"].map((name) =>
        field(add, name)?.getAttribute("placeholder")
      )
    ).toEqual([
      "Morning summary",
      "08:00",
      "6",
      "Summarize what changed in the repository since yesterday.",
    ])
  })

  it("asks before removing a routine, and sends the removal only on a yes", async () => {
    draw(
      projectOf({
        Routines: [
          {
            ID: "r1",
            Title: "Morning",
            Prompt: "go",
            Thread: "3",
            When: "every hour",
            Last: "not yet",
            Paused: false,
          },
        ],
      })
    )
    const sent = vi.spyOn(globalThis, "fetch")
    fireEvent.click(await screen.findByRole("button", { name: "Remove" }))
    const asking = await screen.findByRole("alertdialog")
    expect(
      within(asking).getByText("Remove this routine? Its conversation is kept.")
    ).toBeTruthy()
    expect(sent.mock.calls.some(([, init]) => init?.method === "POST")).toBe(
      false
    )
    fireEvent.click(within(asking).getByTestId("confirm"))
    await vi.waitFor(() =>
      expect(
        sent.mock.calls.some(
          ([path, init]) =>
            init?.method === "POST" &&
            String(path).endsWith("/routines/r1/remove")
        )
      ).toBe(true)
    )
  })

  it("asks before removing the agent, and says its files are kept aside", async () => {
    draw(projectOf())
    const remove = await screen.findByTestId("remove-agent")
    expect(remove.getAttribute("action")).toBe("/instances/atlas-1/remove")
    expect(
      screen.getByText("Its files are kept aside, not deleted.")
    ).toBeTruthy()
    fireEvent.click(
      within(remove).getByRole("button", { name: "Remove this agent" })
    )
    expect(
      within(await screen.findByRole("alertdialog")).getByText(
        "Remove this agent? Its files are kept aside and its activity stays in the log."
      )
    ).toBeTruthy()
  })

  it("shares the apps the agent built, linking only one that answers", async () => {
    draw(
      projectOf({
        Sharing: true,
        Runs: true,
        Apps: [
          {
            ID: "a1",
            State: "live",
            Link: "/apps/a1/",
            Folder: "",
            Command: "node server.js",
            Note: "",
            Port: 8080,
          },
          {
            ID: "a2",
            State: "failed",
            Link: "/apps/a2/",
            Folder: "",
            Command: "node a.js",
            Note: "Error: Cannot find module 'express'",
            Port: 3000,
          },
          {
            ID: "a3",
            State: "starting",
            Link: "/apps/a3/",
            Folder: "site",
            Command: "",
            Note: "",
            Port: 0,
          },
        ],
      })
    )
    const [live, failed, starting] = within(
      await screen.findByTestId("apps")
    ).getAllByTestId("app")
    const open = within(live).getByRole("link", { name: "Open the app" })
    expect([open.getAttribute("href"), open.getAttribute("target")]).toEqual([
      "/apps/a1/",
      "_blank",
    ])
    expect(live.textContent).toContain("node server.js on 8080")
    expect(
      within(live)
        .getByRole("button", { name: "Stop" })
        .closest("form")!
        .getAttribute("action")
    ).toBe("/instances/atlas-1/apps/a1/remove")
    expect(failed.textContent).toContain("It did not start")
    expect(failed.textContent).toContain("Cannot find module 'express'")
    expect(within(failed).queryByRole("link")).toBeNull()
    expect(within(failed).getByRole("button", { name: "Dismiss" })).toBeTruthy()
    expect(starting.textContent).toContain("Starting…the page in site")
    fireEvent.click(
      screen.getByRole("button", { name: "Share an app this agent built" })
    )
    const share = screen
      .getByRole("button", { name: "Start it and share it" })
      .closest("form")!
    expect(share.getAttribute("action")).toBe("/instances/atlas-1/apps")
    expect(field(share, "port")!.value).toBe("8080")
  })

  it("says a page is shared from its conversation where this server runs no apps", async () => {
    draw(projectOf({ Sharing: true, Runs: false }))
    expect(
      await screen.findByText(
        "A page this agent builds can be opened from the conversation it was built in, with Preview."
      )
    ).toBeTruthy()
    expect(screen.queryByText("Share an app this agent built")).toBeNull()
  })

  it("offers no apps where nothing is shared", async () => {
    draw(projectOf())
    await screen.findByTestId("routines")
    expect(screen.queryByTestId("apps")).toBeNull()
  })

  it("lists its checked tasks and runs another with a check, a sensitivity, a team and a cap", async () => {
    draw(
      projectOf({
        Tasks: [
          {
            ID: "count-1",
            Brief: "count the files",
            State: "passed",
            Queued: 0,
          },
        ],
      })
    )
    const tasks = await screen.findByTestId("tasks")
    expect(
      within(tasks).getByRole("heading", { name: "Checked tasks" })
    ).toBeTruthy()
    expect(
      within(tasks)
        .getByRole("link", { name: /count the files/ })
        .getAttribute("href")
    ).toBe("/jobs/count-1")
    const run = within(tasks)
      .getByRole("button", { name: "Run it" })
      .closest("form")!
    expect(run.getAttribute("action")).toBe("/instances/atlas-1/tasks")
    expect(
      ["brief", "gate", "sensitivity", "team", "usd"].map((name) =>
        Boolean(field(run, name))
      )
    ).toEqual([true, true, true, true, true])
    expect(field(run, "sensitivity")!.value).toBe("internal")
  })

  it("reads itself again while an app is starting", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const { asked } = draw(projectOf({ Sharing: true, Refresh: true }))
    await screen.findByTestId("apps")
    const before = asked.length
    await vi.advanceTimersByTimeAsync(3500)
    expect(asked.length).toBeGreaterThan(before)
    vi.useRealTimers()
  })
})
