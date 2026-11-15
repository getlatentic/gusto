import { usd } from "@/lib/format"

// MemberBudget is a person's month against their budget, as the server counts it.
export interface MemberBudget {
  Set: boolean
  Passed: boolean
  Limit: number
  Spent: number
}

// budgetLine is what a person's own usage says of their budget: what they have spent of it, or that they have reached it and their
// agents are stopped.
export function budgetLine(mine: MemberBudget): {
  reached: boolean
  words: string
} {
  const spent = usd(mine.Spent)
  const limit = usd(mine.Limit)
  if (mine.Passed) {
    return {
      reached: true,
      words: `You have reached your budget for this month: ${spent} of ${limit}. Your agents are stopped until next month, or until an admin raises it.`,
    }
  }
  return {
    reached: false,
    words: `You have spent ${spent} of your ${limit} budget this month.`,
  }
}
