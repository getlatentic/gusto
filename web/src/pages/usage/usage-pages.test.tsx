import { screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import {
  MySessionPage,
  MySpendPage,
  MyUsagePage,
} from "@/pages/usage/usage-pages"
import type {
  SessionData,
  SpendingData,
  UsageData,
  UsageRow,
} from "@/lib/types"
import { renderPage } from "@/test/render"

// A person's own usage is drawn by the same pages as the console's, at their own addresses, from data that holds only theirs.

const row: UsageRow = {
  ID: "chat:1",
  Title: "Draft the policy",
  Href: "/usage/chat:1",
  Record: "/jobs/t1",
  Last: "15 Nov 03:24",
  LastFull: "2026-11-15 03:24:29 CET",
  Agent: "Nova",
  Engine: "Codex",
  Harness: "codex",
  Mark: "",
  MarkDark: "",
  Asked: "Ada",
  Owner: "Ada",
  Models: "m",
  Cost: "no price",
  Unpriced: "",
  Turns: 1,
}

function mine(change: (page: UsageData) => void = () => {}): UsageData {
  const page: UsageData = {
    Base: "/usage",
    Tabs: { Summary: "/spend", Tasks: "/usage", Current: "tasks" },
    Mine: { Set: true, Passed: false, Limit: 20, Spent: 1.5 },
    Usage: {
      Values: {},
      Everyone: false,
      Facets: {
        people: ["ada@acme.com"],
        agentTypes: ["Codex"],
        agents: ["Nova"],
        providers: ["bedrock"],
        models: ["m"],
      },
      Building: { building: false, done: 0, total: 0 },
      Figures: {
        Spent: "no price",
        SpentCaption: "no price set for these models",
        In: "1.0k",
        Out: "100",
        Cache: "0%",
        Unpriced: "",
        Calls: 1,
        Refused: 0,
      },
      Rows: [row],
      Columns: [
        {
          Label: "Last request",
          Href: "/usage?sort=last",
          Sorted: "descending",
          Num: false,
        },
        { Label: "Conversation", Href: "", Sorted: "", Num: false },
        { Label: "Agent · owner", Href: "", Sorted: "", Num: false },
        { Label: "Cost", Href: "/usage?sort=cost", Sorted: "", Num: true },
        { Label: "Model", Href: "", Sorted: "", Num: false },
      ],
      States: [
        { Label: "All", Href: "/usage", Count: "", Current: true },
        {
          Label: "Failed",
          Href: "/usage?state=failed",
          Count: "0 this week",
          Current: false,
        },
      ],
      ShowWho: false,
      Filtered: false,
      Clear: "/usage",
      Newest: "",
      Next: "/usage?after=c1",
      Export: "/api/usage.csv",
    },
  }
  change(page)
  return page
}

const drawUsage = (page: UsageData, at = "/usage") =>
  renderPage(at, "/usage", <MyUsagePage />, { [at]: page })

describe("a person's own usage", () => {
  it("lists their conversations under what they have spent of their budget", async () => {
    drawUsage(mine())
    expect(
      await screen.findByText(
        "You have spent $1.50 of your $20.00 budget this month."
      )
    ).toBeTruthy()
    const link = screen.getByRole("link", { name: "Draft the policy" })
    expect(link.getAttribute("href")).toBe("/usage/chat:1")
    expect(screen.getByTestId("usage-row").textContent).toContain(
      "Nova · Ada's"
    )
    expect(
      screen.getByRole("link", { name: "Older" }).getAttribute("href")
    ).toBe("/usage?after=c1")
  })

  it("has no column or choice of who asked, since every row is theirs", async () => {
    drawUsage(mine())
    await screen.findByTestId("usage-table")
    expect(screen.queryByRole("columnheader", { name: "Asked by" })).toBeNull()
    expect(screen.queryByRole("button", { name: "Person" })).toBeNull()
    for (const facet of ["Agent", "Agent type", "Provider", "Model"]) {
      expect(screen.getByRole("button", { name: facet })).toBeTruthy()
    }
  })

  it("leads to their summary and their conversations, and keeps them in their own place", async () => {
    drawUsage(mine())
    await screen.findByTestId("usage-table")
    const tabs = screen.getByRole("tablist", { name: "Usage views" })
    expect(
      within(tabs).getByRole("tab", { name: "Summary" }).getAttribute("href")
    ).toBe("/spend")
    expect(
      within(tabs)
        .getByRole("tab", { name: "Conversations" })
        .getAttribute("href")
    ).toBe("/usage")
    expect(
      screen
        .getByRole("link", { name: "Cost" })
        .closest("th")
        ?.getAttribute("aria-sort")
    ).toBeNull()
    expect(
      screen
        .getByRole("link", { name: "Last request" })
        .closest("th")
        ?.getAttribute("aria-sort")
    ).toBe("descending")
  })

  it("says when their budget is reached, and that their agents are stopped", async () => {
    drawUsage(
      mine((page) => (page.Mine = { ...page.Mine!, Passed: true, Spent: 20 }))
    )
    expect((await screen.findByRole("alert")).textContent).toBe(
      "You have reached your budget for this month: $20.00 of $20.00. Your agents are stopped until next month, or until an admin raises it."
    )
  })

  it("says nothing of a budget when they have none", async () => {
    drawUsage(mine((page) => (page.Mine = undefined)))
    await screen.findByTestId("usage-table")
    expect(screen.queryByText(/budget this month/)).toBeNull()
  })

  it("says when they have made no model call yet", async () => {
    drawUsage(mine((page) => (page.Usage!.Rows = [])))
    expect(await screen.findByText("No model calls yet")).toBeTruthy()
  })

  it("offers the CSV of what it lists", async () => {
    drawUsage(mine())
    await screen.findByTestId("usage-table")
    expect(screen.getByRole("button", { name: "More" })).toBeTruthy()
  })
})

describe("one of a person's sessions", () => {
  const session: SessionData = {
    Base: "/usage",
    Session: {
      ID: "chat:1",
      Person: "ada@acme.com",
      Agent: "Nova",
      Engine: "",
      Search: "",
      Calls: [
        {
          At: "15 Nov 02:24:29",
          AtFull: "2026-11-15 02:24:29 UTC",
          Task: "t1",
          Record: "/jobs/t1",
          Model: "m",
          Provider: "bedrock",
          Key: "",
          In: "1.0k",
          Out: "100",
          Cache: "—",
          Cost: "no price",
          Refused: "",
          Translated: "",
          Status: 200,
          Unpriced: true,
        },
      ],
      Next: "",
      Newest: "",
    },
  }

  it("lists its calls under the way back to their usage", async () => {
    renderPage("/usage/chat:1", "/usage/:session", <MySessionPage />, {
      "/usage/chat:1": session,
    })
    expect(await screen.findByText(/ada@acme.com, agent Nova\./)).toBeTruthy()
    expect(
      within(screen.getByRole("navigation", { name: "Where this is" }))
        .getByRole("link", { name: "Usage" })
        .getAttribute("href")
    ).toBe("/usage")
    const call = screen.getByTestId("session-call")
    expect(within(call).getByText("no price")).toBeTruthy()
    expect(
      within(call).getByRole("link", { name: "t1" }).getAttribute("href")
    ).toBe("/jobs/t1")
  })

  it("says when its search matches no call", async () => {
    renderPage("/usage/chat:1?q=zzz", "/usage/:session", <MySessionPage />, {
      "/usage/chat:1?q=zzz": {
        ...session,
        Session: { ...session.Session, Search: "zzz", Calls: [] },
      },
    })
    expect(
      await screen.findByText("No call of this session matches.")
    ).toBeTruthy()
    expect(
      screen.getByRole("link", { name: /Reset/ }).getAttribute("href")
    ).toBe("/usage/chat%3A1")
  })
})

describe("a person's own summary", () => {
  const spend: SpendingData = {
    Base: "/spend",
    Tabs: { Summary: "/spend", Tasks: "/usage", Current: "summary" },
    Ledger: {
      month: "2026-11",
      total: {
        calls: 3,
        usd: 0.75,
        unmeasured: 1,
        uncounted: 0,
        inputTokens: 3000,
        cachedTokens: 1200,
        outputTokens: 600,
        refused: 0,
      },
      Months: ["2026-10", "2026-11"],
      Measure: "money",
      Daily: [{ Date: "2026-11-01", Label: "1", Height: 100, Says: "$0.75" }],
      DayLabels: ["1 Nov", "30 Nov"],
      Tops: [{ Title: "Biggest team", Name: "platform", Says: "$0.50" }],
    },
  }

  it("says what their tasks cost this month, and that some calls have no price", async () => {
    renderPage("/spend", "/spend", <MySpendPage />, { "/spend": spend })
    const spent = await screen.findByTestId("spend-spent")
    expect(within(spent).getByTestId("figure-value").textContent).toBe(
      "$0.7500"
    )
    expect(spent.getAttribute("title")).toBe(
      "at least: some calls have no price"
    )
    expect(
      within(screen.getByTestId("spend-calls")).getByTestId("figure-value")
        .textContent
    ).toBe("3")
    expect(screen.getByText("platform")).toBeTruthy()
    expect(
      screen.getByRole("combobox", { name: "Month" }).textContent
    ).toContain("2026-11")
    expect(
      screen.getByRole("tab", { name: "Conversations" }).getAttribute("href")
    ).toBe("/usage")
  })
})
