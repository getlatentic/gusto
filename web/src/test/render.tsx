import type { ReactNode } from "react"
import { render } from "@testing-library/react"
import { QueryClientProvider } from "@tanstack/react-query"
import { MemoryRouter, Route, Routes } from "react-router"
import { vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { newQueryClient } from "@/lib/api"

// renderPage draws a page at an address, with the server answering /app/api from the data given, keyed by the page address.
export function renderPage(
  at: string,
  route: string,
  page: ReactNode,
  data: Record<string, unknown>
) {
  const asked: string[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string) => {
      asked.push(path)
      const key = path.replace(/^\/app\/api/, "")
      if (!(key in data)) return new Response("not found", { status: 404 })
      return new Response(JSON.stringify(data[key]), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    })
  )
  const drawn = render(
    <QueryClientProvider client={testQueryClient()}>
      <TooltipProvider>
        <MemoryRouter initialEntries={[at]}>
          <Routes>
            <Route path={route} element={page} />
          </Routes>
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>
  )
  return { ...drawn, asked }
}

// testQueryClient is a fresh cache for one test, which asks once and reports a failure at once.
export function testQueryClient() {
  const client = newQueryClient()
  client.setDefaultOptions({ queries: { retry: false } })
  return client
}
