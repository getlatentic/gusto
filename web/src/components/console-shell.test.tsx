import { render, screen } from "@testing-library/react"
import { MemoryRouter, Route, Routes } from "react-router"
import { describe, expect, it, vi } from "vitest"

import { ConsoleShell } from "@/components/console-shell"
import { TooltipProvider } from "@/components/ui/tooltip"

describe("the console", () => {
  it("has its sections and one way back to the pages everybody uses, and no other", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              sections: [
                { key: "overview", href: "/admin", label: "Overview" },
                { key: "budgets", href: "/admin/budgets", label: "Budgets" },
              ],
            })
          )
      )
    )
    render(
      <TooltipProvider>
        <MemoryRouter initialEntries={["/admin/budgets"]}>
          <Routes>
            <Route path="/admin" element={<ConsoleShell />}>
              <Route path="budgets" element={<p>budgets</p>} />
            </Route>
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    )
    const current = await screen.findByRole("link", { name: "Budgets" })
    expect(current.getAttribute("aria-current")).toBe("page")
    const links = screen
      .getAllByRole("link")
      .map((link) => link.getAttribute("href"))
    expect(links.filter((href) => !href?.startsWith("/admin"))).toEqual(["/"])
    expect(screen.getByRole("link", { name: "Back to gusto" })).toBeTruthy()
  })
})
