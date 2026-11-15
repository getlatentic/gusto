import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { UsagePage } from "@/pages/admin/usage"
import { renderPage } from "@/test/render"
import type { UsageData, UsageView } from "@/lib/types"

function usage(change: (view: UsageView) => void): UsageData {
  const view: UsageView = {
    Values: {},
    Everyone: true,
    Facets: {
      people: ["ada@acme.com"],
      agentTypes: ["Codex"],
      agents: ["Nova"],
      providers: ["bedrock"],
      models: ["m"],
    },
    Building: { building: false, done: 0, total: 0 },
    Figures: {
      Spent: "$1.25",
      SpentCaption: "spent, all time",
      In: "4.0k",
      Out: "400",
      Cache: "10%",
      Unpriced: "",
      Calls: 4,
      Refused: 0,
    },
    Rows: [],
    Columns: [
      {
        Label: "Last request",
        Href: "/admin/usage?sort=last",
        Sorted: "descending",
        Num: false,
      },
      { Label: "Conversation", Href: "", Sorted: "", Num: false },
      { Label: "Asked by", Href: "", Sorted: "", Num: false },
      { Label: "Agent · owner", Href: "", Sorted: "", Num: false },
      { Label: "Cost", Href: "/admin/usage?sort=cost", Sorted: "", Num: true },
      { Label: "Model", Href: "", Sorted: "", Num: false },
    ],
    States: [
      { Label: "All", Href: "/admin/usage", Count: "", Current: true },
      {
        Label: "Failed",
        Href: "/admin/usage?state=failed",
        Count: "2 this week",
        Current: false,
      },
    ],
    ShowWho: true,
    Filtered: false,
    Clear: "/admin/usage",
    Newest: "",
    Next: "",
    Export: "/api/usage.csv",
  }
  change(view)
  return {
    Base: "/admin/usage",
    Tabs: {
      Summary: "/admin/spending",
      Tasks: "/admin/usage",
      Current: "tasks",
    },
    Usage: view,
  }
}

const row = {
  ID: "room:1",
  Title: "Draft the policy",
  Href: "/admin/usage/room:1",
  Record: "/jobs/t1",
  Last: "15 Nov 03:24",
  LastFull: "2026-11-15 03:24:29 CET",
  Agent: "Nova",
  Engine: "Codex",
  Harness: "codex",
  Mark: "",
  MarkDark: "",
  Asked: "Bob",
  Owner: "Alice",
  Models: "m",
  Cost: "$1.25",
  Unpriced: "",
  Turns: 2,
}

describe("the usage", () => {
  it("lists each conversation: who asked, whose agent answered, and what it cost", async () => {
    renderPage("/admin/usage", "/admin/usage", <UsagePage />, {
      "/admin/usage": usage((view) => (view.Rows = [row])),
    })
    expect(
      (
        await screen.findByRole("link", { name: "Draft the policy" })
      ).getAttribute("href")
    ).toBe("/admin/usage/room:1")
    expect(screen.getByText("Bob")).toBeTruthy()
    expect(screen.getByText(/Nova/).textContent).toContain("Nova · Alice's")
    expect(screen.getByText("· 2 turns")).toBeTruthy()
    expect(screen.getByRole("link", { name: /Failed/ }).textContent).toBe(
      "Failed 2 this week"
    )
  })

  it("picks its days from a calendar, not the browser's own date field", async () => {
    renderPage("/admin/usage", "/admin/usage", <UsagePage />, {
      "/admin/usage": usage((view) => (view.Rows = [row])),
    })
    await screen.findByRole("link", { name: "Draft the policy" })
    expect(document.querySelector('input[type="date"]')).toBeNull()
    expect(screen.getAllByRole("button", { name: "Any day" })).toHaveLength(2)
  })

  it("says when no conversation has failed", async () => {
    renderPage("/admin/usage?state=failed", "/admin/usage", <UsagePage />, {
      "/admin/usage?state=failed": usage(
        (view) => (view.Values = { state: ["failed"] })
      ),
    })
    expect(await screen.findByText("No conversation has failed")).toBeTruthy()
  })

  it("says when its filters match nothing, with the way to clear them", async () => {
    renderPage("/admin/usage?q=zzz", "/admin/usage", <UsagePage />, {
      "/admin/usage?q=zzz": usage((view) => {
        view.Values = { q: ["zzz"] }
        view.Filtered = true
      }),
    })
    expect(
      await screen.findByText("No session matches these filters")
    ).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "Clear them" }).getAttribute("href")
    ).toBe("/admin/usage")
  })
})
