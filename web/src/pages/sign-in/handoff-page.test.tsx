import { screen, waitFor } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { leaveFor } from "@/lib/leave"
import { HandoffPage } from "@/pages/sign-in/handoff-page"
import { renderPage } from "@/test/render"

vi.mock("@/lib/leave", () => ({ leaveFor: vi.fn() }))

const link = "latentic://signed-in?ticket=abc"

describe("the desktop app's hand-off", () => {
  it("sends the browser back to the app with its ticket, once, and links to it too", async () => {
    const { asked } = renderPage(
      "/auth/desktop/handoff",
      "/auth/desktop/handoff",
      <HandoffPage />,
      { "/auth/desktop/handoff": { Link: link } }
    )
    expect(screen.getByRole("heading").textContent).toBe("You are signed in")
    expect(
      screen.getByText("Return to gusto to carry on. You can close this tab.")
    ).toBeTruthy()
    await waitFor(() => expect(leaveFor).toHaveBeenCalledWith(link))
    expect(screen.getByTestId("open-app").getAttribute("href")).toBe(link)
    expect(screen.getByTestId("open-app").textContent).toBe("Open gusto")
    expect(asked).toEqual(["/app/api/auth/desktop/handoff"])
  })

  it("says so when the ticket was already handed over", async () => {
    renderPage(
      "/auth/desktop/handoff",
      "/auth/desktop/handoff",
      <HandoffPage />,
      {}
    )
    expect((await screen.findByRole("alert")).textContent).toContain(
      "already handed to the app"
    )
  })
})
