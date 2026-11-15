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

  it("marks the section a tab of a page belongs to", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              sections: [
                { key: "overview", href: "/admin", label: "Overview" },
                { key: "records", href: "/admin/records", label: "Records" },
              ],
            })
          )
      )
    )
    render(
      <TooltipProvider>
        <MemoryRouter initialEntries={["/admin/records?tab=keeping"]}>
          <Routes>
            <Route path="/admin" element={<ConsoleShell />}>
              <Route path="records" element={<p>records</p>} />
            </Route>
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    )
    const current = await screen.findByRole("link", { name: "Records" })
    expect(current.getAttribute("aria-current")).toBe("page")
    expect(
      screen
        .getByRole("link", { name: "Overview" })
        .getAttribute("aria-current")
    ).toBeNull()
  })
})

describe("a page under a section", () => {
  it("marks Apps & skills for a skill set's page and a plugin's", async () => {
    for (const at of [
      "/admin/skill-sets/reviewer",
      "/admin/integrations/reports",
    ]) {
      vi.stubGlobal(
        "fetch",
        vi.fn(
          async () =>
            new Response(
              JSON.stringify({
                sections: [
                  { key: "overview", href: "/admin", label: "Overview" },
                  { key: "apps", href: "/admin/apps", label: "Apps & skills" },
                ],
              })
            )
        )
      )
      const drawn = render(
        <TooltipProvider>
          <MemoryRouter initialEntries={[at]}>
            <Routes>
              <Route path="/admin" element={<ConsoleShell />}>
                <Route path="*" element={<p>page</p>} />
              </Route>
            </Routes>
          </MemoryRouter>
        </TooltipProvider>
      )
      const apps = await screen.findByRole("link", { name: "Apps & skills" })
      expect(apps.getAttribute("aria-current")).toBe("page")
      expect(
        screen
          .getByRole("link", { name: "Overview" })
          .getAttribute("aria-current")
      ).toBeNull()
      drawn.unmount()
    }
  })
})
