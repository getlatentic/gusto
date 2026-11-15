import { describe, expect, it } from "vitest"

import { budgetLine } from "@/domain/usage-budget"

describe("a person's budget line", () => {
  it("says what they have spent of it", () => {
    expect(
      budgetLine({ Set: true, Passed: false, Limit: 20, Spent: 0 })
    ).toEqual({
      reached: false,
      words: "You have spent $0.00 of your $20.00 budget this month.",
    })
  })

  it("says when it is reached, and that their agents are stopped", () => {
    expect(
      budgetLine({ Set: true, Passed: true, Limit: 20, Spent: 20.5 })
    ).toEqual({
      reached: true,
      words:
        "You have reached your budget for this month: $20.50 of $20.00. Your agents are stopped until next month, or until an admin raises it.",
    })
  })

  it("writes fractions of a cent until there are dollars, as the headlines do", () => {
    expect(
      budgetLine({ Set: true, Passed: false, Limit: 5, Spent: 0.0042 }).words
    ).toBe("You have spent $0.0042 of your $5.00 budget this month.")
  })
})
