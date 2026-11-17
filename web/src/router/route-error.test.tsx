import { render, screen } from "@testing-library/react"
import { createMemoryRouter } from "react-router"
import { RouterProvider } from "react-router/dom"
import { describe, expect, it } from "vitest"

import { RouteError } from "@/router/route-error"

function draw(lazy: () => Promise<never>, at = "/jobs") {
  const router = createMemoryRouter(
    [{ errorElement: <RouteError />, children: [{ path: "/jobs", lazy }] }],
    { initialEntries: [at] }
  )
  render(<RouterProvider router={router} />)
}

describe("a page that cannot be shown", () => {
  it("says so and offers to reload when its code does not load", async () => {
    draw(() => Promise.reject(new Error("Failed to fetch module")))
    const shown = await screen.findByTestId("route-error")
    expect(shown.textContent).toContain("This page could not be shown.")
    expect(screen.getByRole("button", { name: "Reload" })).toBeTruthy()
  })

  it("says there is nothing at an address the app has no page for", async () => {
    draw(() => Promise.reject(new Error("unused")), "/nowhere")
    expect((await screen.findByRole("heading")).textContent).toBe(
      "There is nothing here."
    )
  })
})
