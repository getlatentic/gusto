import { fireEvent, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { BudgetsPage, type BudgetsData } from "@/pages/admin/budgets"
import { renderPage } from "@/test/render"

function budgets(change: (page: BudgetsData) => void): BudgetsData {
  const page: BudgetsData = {
    Budget: {
      Editable: true,
      Set: true,
      Limit: 50,
      Passed: false,
      Err: "",
      Member: 10,
      MemberSet: true,
    },
    People: [
      {
        Email: "ada@acme.com",
        Name: "Ada",
        Percent: 100,
        Budget: {
          Set: true,
          Own: true,
          Limit: 4,
          Spent: 5,
          Passed: true,
          Who: "ada@acme.com",
        },
      },
      {
        Email: "bob@acme.com",
        Name: "Bob",
        Percent: 0,
        Budget: {
          Set: true,
          Own: false,
          Limit: 10,
          Spent: 0,
          Passed: false,
          Who: "bob@acme.com",
        },
      },
    ],
    Pages: { Shown: "", Previous: "", Next: "" },
    Listed: true,
    Refused: "",
  }
  change(page)
  return page
}

const draw = (page: BudgetsData, at = "/admin/budgets") =>
  renderPage(at, "/admin/budgets", <BudgetsPage />, { [at]: page })

describe("budgets", () => {
  it("says both budgets in a line, and each person's against what they spent", async () => {
    draw(budgets(() => {}))
    expect(
      await screen.findByText(
        "Each person: $10.00 a month; their agents stop there. Organization: $50.00 a month, which only warns."
      )
    ).toBeTruthy()
    const ada = screen.getByText("ada@acme.com").closest("tr")!
    expect(ada.textContent).toContain("$4.00own")
    expect(ada.textContent).toContain("reached")
    const bob = screen.getByText("bob@acme.com").closest("tr")!
    expect(bob.textContent).toContain("nothing")
    expect(bob.textContent).not.toContain("own")
  })

  it("sets both budgets in one form behind Change, holding what is kept", async () => {
    draw(budgets(() => {}))
    fireEvent.click(await screen.findByRole("button", { name: "Change" }))
    const form = (await screen.findByRole("dialog")).querySelector("form")!
    expect(form.getAttribute("action")).toBe("/admin/budget")
    expect(
      (screen.getByLabelText("Each person, a month") as HTMLInputElement).value
    ).toBe("10.00")
    expect(
      (screen.getByLabelText("Organization, a month") as HTMLInputElement).value
    ).toBe("50.00")
  })

  it("keeps one person's own budget behind the ⋯ on their row", async () => {
    draw(budgets(() => {}))
    fireEvent.click(await screen.findByRole("button", { name: "More for Bob" }))
    const field = (await screen.findByLabelText(
      "Their own budget, a month"
    )) as HTMLInputElement
    expect(field.placeholder).toBe("10.00, as everyone")
    expect(field.value).toBe("")
    expect(field.form?.getAttribute("action")).toBe("/admin/member-budget")
  })

  it("says why the budgets could not be read, and offers no form", async () => {
    draw(
      budgets((page) => {
        page.Budget.Err = "the file is damaged"
      })
    )
    expect(
      await screen.findByText(
        "The budgets could not be read. the file is damaged"
      )
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Change" })).toBeNull()
  })

  it("shows a refusal", async () => {
    draw(
      budgets((page) => {
        page.Refused = "the budget is a number of dollars"
      })
    )
    expect((await screen.findByRole("alert")).textContent).toBe(
      "the budget is a number of dollars"
    )
  })
})
