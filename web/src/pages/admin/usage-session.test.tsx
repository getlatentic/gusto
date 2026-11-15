import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { UsageSessionPage } from "@/pages/admin/usage-session"
import { renderPage } from "@/test/render"
import type { SessionData } from "@/lib/types"

function session(search: string): SessionData {
  return {
    Base: "/admin/usage",
    Session: {
      ID: "room:1",
      Person: "ada@acme.com",
      Agent: "Nova",
      Engine: "Codex",
      Search: search,
      Calls: [
        {
          At: "15 Nov 02:24:29",
          AtFull: "2026-11-15 02:24:29 UTC",
          Task: "t1",
          Record: "/jobs/t1",
          Model: "m",
          Provider: "bedrock",
          Key: "team",
          In: "1.0k",
          Out: "100",
          Cache: "10%",
          Cost: "$0.25",
          Refused: "",
          Translated: "",
          Status: 200,
          Unpriced: false,
        },
      ],
      Next: "",
      Newest: "",
    },
  }
}

describe("a session", () => {
  it("searches its calls as the person types, with no button to press", async () => {
    renderPage(
      "/admin/usage/room:1",
      "/admin/usage/:session",
      <UsageSessionPage />,
      {
        "/admin/usage/room:1": session(""),
      }
    )
    expect(
      await screen.findByRole("searchbox", {
        name: "Search this session's calls",
      })
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Search" })).toBeNull()
    expect(screen.queryByRole("link", { name: /Reset/ })).toBeNull()
    expect(screen.getByRole("link", { name: "t1" }).getAttribute("href")).toBe(
      "/jobs/t1"
    )
  })

  it("keeps what was searched for, and offers to reset it", async () => {
    renderPage(
      "/admin/usage/room:1?q=team",
      "/admin/usage/:session",
      <UsageSessionPage />,
      {
        "/admin/usage/room:1?q=team": session("team"),
      }
    )
    const box = (await screen.findByRole("searchbox")) as HTMLInputElement
    expect(box.value).toBe("team")
    expect(
      screen.getByRole("link", { name: /Reset/ }).getAttribute("href")
    ).toBe("/admin/usage/room%3A1")
  })
})
