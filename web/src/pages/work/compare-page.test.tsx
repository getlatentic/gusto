import { screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { ComparePage } from "@/pages/work/compare-page"
import { EnginesPage } from "@/pages/work/engines-page"
import { compareOf, lineupOf, sideOf } from "@/pages/work/fixture"
import { LineupPage } from "@/pages/work/lineup-page"
import type { CompareData, EnginesData, LineupData } from "@/pages/work/types"
import { renderPage } from "@/test/render"

const drawCompare = (page: CompareData) =>
  renderPage("/compare", "/compare", <ComparePage />, { "/compare": page })

const drawLineup = (page: LineupData) =>
  renderPage("/compare/two-runs", "/compare/:id", <LineupPage />, {
    "/compare/two-runs": page,
  })

describe("comparing agents", () => {
  it("gives each agent that can take the sample as a choice of its own, all chosen, on a model", async () => {
    drawCompare(compareOf())
    const form = await screen.findByTestId("start-comparison")
    expect(form.getAttribute("action")).toBe("/compare")
    expect(
      (form.querySelector('[name="brief"]') as HTMLTextAreaElement).value
    ).toBe("Run the tests and fix what fails")
    expect(form.textContent).toContain(
      "On the Engineering sample project, checked by running node --test after each agent."
    )
    const engines = form.querySelectorAll("[data-engine]")
    expect([...engines].map((engine) => engine.textContent)).toEqual([
      "Codex",
      "Pi",
    ])
    expect(
      within(form)
        .getAllByRole("checkbox")
        .map((box) => box.getAttribute("aria-checked"))
    ).toEqual(["true", "true"])
    expect(
      [...form.querySelectorAll('input[name="harness"]')].map(
        (box) => (box as HTMLInputElement).value
      )
    ).toEqual(["codex", "pi"])
    expect(
      (form.querySelector('[name="model"]') as HTMLInputElement).value
    ).toBe("big-1")
  })

  it("says why a comparison was refused, and lists earlier ones", async () => {
    drawCompare(
      compareOf({
        Refused: "choose at least two agents to compare",
        Lineups: [
          {
            ID: "l1",
            Brief: "fix the parser",
            Model: "big-1",
            Agents: 2,
            At: 0,
            When: "15 Nov 10:00",
          },
        ],
      })
    )
    expect((await screen.findByRole("alert")).textContent).toBe(
      "choose at least two agents to compare"
    )
    const earlier = screen.getByRole("region", { name: "Earlier comparisons" })
    expect(
      within(earlier)
        .getByRole("link", { name: "fix the parser" })
        .getAttribute("href")
    ).toBe("/compare/l1")
    expect(within(earlier).getAllByRole("row")[1].textContent).toBe(
      "fix the parserbig-12"
    )
  })

  it("says there is nothing to compare on without a sample that has a check", async () => {
    drawCompare(
      compareOf({ Template: undefined, Engines: undefined, Models: undefined })
    )
    expect(await screen.findByText("Nothing to compare on yet")).toBeTruthy()
    expect(
      screen.getByText(
        "No template here has a check that can decide between agents."
      )
    ).toBeTruthy()
    expect(screen.queryByTestId("start-comparison")).toBeNull()
  })
})

describe("one comparison", () => {
  it("says which did better once every run is over, with each run's figures and details", async () => {
    drawLineup(lineupOf())
    expect((await screen.findByTestId("lineup-reading")).textContent).toBe(
      "Codex was the only one to pass the check."
    )
    expect(
      screen.getByText(
        "Same code, same model (big-1), same skills, same check. Only the agent differs."
      )
    ).toBeTruthy()
    expect(screen.getByTestId("lineup-passed").parentElement!.textContent).toBe(
      "1 of 2passed the tests"
    )
    const [codex, pi] = screen.getAllByTestId("side")
    expect(
      [...codex.querySelectorAll("td")].map((cell) => cell.textContent!.trim())
    ).toEqual([
      "Codexbig-1",
      "Tests passFastest",
      "40 s",
      "$0.7500",
      "3",
      "1 file +2 −1",
      "Details",
    ])
    expect(
      within(codex).getByRole("link", { name: "Details" }).getAttribute("href")
    ).toBe("/jobs/run-1")
    expect(pi.textContent).toContain("Tests fail")
    expect(screen.getByText(/Figures within 15% of each other/)).toBeTruthy()
    expect(screen.getByTestId("page-title").textContent).toBe("fix it")
  })

  it("names no winner while a run is still working, and says what each waits for", async () => {
    drawLineup(
      lineupOf({
        Running: true,
        Reading: null,
        Sides: [
          sideOf({
            Done: false,
            Holding: "Number 1 in line.",
            Metered: false,
            Seconds: 0,
            Files: 0,
            Fastest: false,
          }),
          sideOf({
            Task: "run-2",
            Done: false,
            Metered: false,
            Seconds: 0,
            Files: 0,
            Fastest: false,
          }),
          sideOf({
            Task: "run-3",
            Checked: false,
            Passed: false,
            Fastest: false,
          }),
        ],
      })
    )
    expect(
      (await screen.findByTestId("lineup-passed")).parentElement!.textContent
    ).toBe("1 of 3passed so far, the rest still working")
    const [holding, working, stopped] = screen.getAllByTestId("side")
    expect(holding.textContent).toContain("Number 1 in line.")
    expect(working.textContent).toContain("Working…")
    expect(stopped.textContent).toContain("Stopped before the tests")
    expect(screen.queryByTestId("lineup-reading")).toBeNull()
    expect(screen.queryByText(/Figures within 15%/)).toBeNull()
  })
})

describe("how each agent type did", () => {
  const draw = (page: EnginesData) =>
    renderPage("/engines", "/engines", <EnginesPage />, { "/engines": page })

  it("is a grid of each agent type on each model", async () => {
    draw({
      Measured: {
        Models: ["big-1", "small-1"],
        Rows: [
          {
            Name: "codex",
            Title: "Codex",
            Mark: "/static/agents/codex.svg",
            MarkDark: "",
            Cells: [
              { Tried: true, Finished: true, Took: "42 s", Cost: "$0.0123" },
              { Tried: true, Finished: false, Took: "90 s", Cost: "" },
            ],
          },
          {
            Name: "pi",
            Title: "Pi",
            Mark: "",
            MarkDark: "",
            Cells: [
              { Tried: false, Finished: false, Took: "", Cost: "" },
              { Tried: true, Finished: true, Took: "30 s", Cost: "" },
            ],
          },
        ],
        Finished: 2,
        Tried: 3,
      },
    })
    const [codex, pi] = await screen.findAllByTestId("engine-row")
    expect(
      [...codex.querySelectorAll("td")].map((cell) => cell.textContent!.trim())
    ).toEqual(["Codex", "Finished42 s · $0.0123", "Did not finish90 s"])
    expect(pi.textContent).toContain("not tried")
    expect(screen.getByText("2 of 3")).toBeTruthy()
    expect(
      screen.getByText(
        "Time includes starting the box. Cost is what the proxy counted."
      )
    ).toBeTruthy()
  })

  it("says when nothing has been measured", async () => {
    draw({ Measured: { Models: null, Rows: null, Finished: 0, Tried: 0 } })
    expect(
      await screen.findByText("Nothing has been measured on this server yet.")
    ).toBeTruthy()
  })
})
