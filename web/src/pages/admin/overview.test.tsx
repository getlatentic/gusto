import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { OverviewPage } from "@/pages/admin/overview"
import { renderPage } from "@/test/render"
import type { OverviewData } from "@/lib/types"

function month(change: (page: OverviewData) => void): OverviewData {
  const page: OverviewData = {
    MonthName: "November",
    Budget: { Set: false, Limit: 0, Passed: false, Over: 0, Err: "" },
    BudgetsPage: true,
    PeopleAll: "/admin/budgets",
    Spent: {
      USD: 0,
      Unmeasured: 0,
      Split: true,
      Building: false,
      People: [],
      Engines: [],
      AllUsage: "/admin/usage?from=2026-11-01",
    },
  }
  change(page)
  return page
}

const draw = (page: OverviewData) =>
  renderPage("/admin", "/admin", <OverviewPage />, { "/admin": page })

describe("the overview", () => {
  it("says no budget is set, and offers to set one", async () => {
    draw(month(() => {}))
    const none = await screen.findByRole("link", { name: "None" })
    expect(none.getAttribute("href")).toBe("/admin/budgets")
    expect(none.closest("[title]")!.getAttribute("title")).toBe(
      "no budget is set"
    )
  })

  it("says the budget is reached when the month spends all of it", async () => {
    draw(
      month((page) => {
        page.Budget = { Set: true, Limit: 100, Passed: true, Over: 0, Err: "" }
        page.Spent.USD = 100
      })
    )
    const alert = await screen.findByRole("alert")
    expect(alert.textContent).toContain(
      "The budget for November is $100.00. $100.00 has been spent, so the budget is reached."
    )
  })

  it("says by how much the budget is passed, and at least, when some calls have no price", async () => {
    draw(
      month((page) => {
        page.Budget = {
          Set: true,
          Limit: 100,
          Passed: true,
          Over: 30.5,
          Err: "",
        }
        page.Spent.USD = 130.5
        page.Spent.Unmeasured = 2
      })
    )
    const alert = await screen.findByRole("alert")
    expect(alert.textContent).toContain(
      "$130.50 has been spent, and more that has no price, so the budget is passed by $30.50 at least."
    )
    expect(
      screen.getByTitle("at least: 2 calls have no price").textContent
    ).toContain("$130.50")
  })

  it("says when the budget cannot be read, without saying it is passed", async () => {
    draw(
      month((page) => {
        page.Budget.Err = "the settings are damaged"
      })
    )
    expect(
      await screen.findByText(
        /The budget could not be read, so it is not checked\./
      )
    ).toBeTruthy()
    expect(screen.queryByText(/so the budget is/)).toBeNull()
  })

  it("lists each person and agent type, with the way to all of them", async () => {
    draw(
      month((page) => {
        page.Spent.People = [
          {
            Label: "Ada",
            Sub: "",
            Href: "",
            USD: 2,
            Calls: 3,
            Unpriced: false,
            Budget: 10,
            Percent: 20,
            More: 0,
          },
          {
            Label: "Cy",
            Sub: "",
            Href: "",
            USD: 0,
            Calls: 0,
            Unpriced: false,
            Budget: 10,
            Percent: 0,
            More: 0,
          },
        ]
        page.Spent.Engines = [
          {
            Label: "Codex",
            Sub: "1 agent",
            Href: "/admin/usage?agent_type=Codex",
            USD: 1,
            Calls: 1,
            Unpriced: false,
            Budget: 0,
            Percent: 0,
            More: 0,
          },
        ]
      })
    )
    expect(await screen.findByText("Ada")).toBeTruthy()
    expect(screen.getByText("$2.00")).toBeTruthy()
    expect(screen.getByText("Nothing yet")).toBeTruthy()
    expect(
      screen
        .getByRole("link", { name: /Codex\s*·\s*1 agent/ })
        .getAttribute("href")
    ).toBe("/admin/usage?agent_type=Codex")
    expect(
      screen
        .getAllByRole("link", { name: "View all" })
        .map((link) => link.getAttribute("href"))
    ).toEqual(["/admin/budgets", "/admin/usage?from=2026-11-01"])
  })
})
