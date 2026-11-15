import { fireEvent, screen, waitFor } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { TokensPage, type TokensData } from "@/pages/admin/tokens"
import { renderPage } from "@/test/render"

const offered = [
  { Scope: "submit", Words: "send tasks" },
  { Scope: "spend", Words: "read spending" },
]

const ci = {
  ID: "abcdef12",
  Name: "ci",
  May: "send tasks, read spending",
  Made: "1 Nov 2026 10:00 UTC",
  MadeDay: "1 Nov",
  By: "ada@acme.com",
  Ended: "",
  LastUsed: "",
  UsedAt: 0,
  Live: true,
}

describe("API tokens", () => {
  it("lists each token with what it may do, and a way to revoke it", async () => {
    renderPage("/admin/tokens", "/admin/tokens", <TokensPage />, {
      "/admin/tokens": { Tokens: [ci], Offered: offered, Refused: "" },
    })
    const row = (await screen.findByText("ci")).closest("tr")!
    expect(row.textContent).toContain("send tasks")
    expect(row.textContent).toContain("Never")
    expect(
      screen.getByRole("button", { name: "Revoke" }).closest("form")!.action
    ).toContain("/admin/tokens/abcdef12/revoke")
    expect(screen.queryByLabelText("Name")).toBeNull()
  })

  it("makes a token in the page and shows its secret there, never in an address", async () => {
    const page: TokensData = { Tokens: [], Offered: offered, Refused: "" }
    const { asked } = renderPage(
      "/admin/tokens",
      "/admin/tokens",
      <TokensPage />,
      { "/admin/tokens": page }
    )
    const answered = vi.mocked(fetch)
    const read = answered.getMockImplementation()!
    answered.mockImplementation(async (path, init) => {
      if (init?.method === "POST") {
        expect(String(init.body)).toBe("name=nightly&scope=submit")
        return new Response(
          JSON.stringify({ Made: "nightly", Secret: "gusto_secret" }),
          { status: 200 }
        )
      }
      return read(path, init)
    })
    fireEvent.change(await screen.findByLabelText("Name"), {
      target: { value: "nightly" },
    })
    fireEvent.click(screen.getByRole("button", { name: "Make token" }))
    expect(await screen.findByText("gusto_secret")).toBeTruthy()
    await waitFor(() =>
      expect(
        asked.filter((path) => path === "/app/api/admin/tokens")
      ).toHaveLength(2)
    )
    expect(asked.some((path) => path.includes("gusto_secret"))).toBe(false)
  })

  it("says why a token was refused, and keeps the form", async () => {
    renderPage("/admin/tokens", "/admin/tokens", <TokensPage />, {
      "/admin/tokens": { Tokens: [], Offered: offered, Refused: "" },
    })
    const read = vi.mocked(fetch).getMockImplementation()!
    vi.mocked(fetch).mockImplementation(async (path, init) =>
      init?.method === "POST"
        ? new Response("a token's name is lower case", { status: 400 })
        : read(path, init)
    )
    fireEvent.change(await screen.findByLabelText("Name"), {
      target: { value: "x" },
    })
    fireEvent.click(screen.getByRole("button", { name: "Make token" }))
    expect((await screen.findByRole("alert")).textContent).toBe(
      "a token's name is lower case"
    )
    expect(screen.getByLabelText("Name")).toBeTruthy()
  })
})
