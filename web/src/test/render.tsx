import type { ReactNode } from "react"
import { render } from "@testing-library/react"
import { Provider } from "react-redux"
import { createMemoryRouter, type RouteObject } from "react-router"
import { RouterProvider } from "react-router/dom"
import { vi } from "vitest"

import { TooltipProvider } from "@/components/ui/tooltip"
import { makeStore, type AppStore } from "@/store"

// renderPage draws a page at an address, with the server answering /app/api from the data given, keyed by the page address.
export function renderPage(
  at: string,
  route: string,
  page: ReactNode,
  data: Record<string, unknown>
) {
  return renderRoutes(at, () => ({ path: route, element: page }), data)
}

// renderRoutes draws the route given at an address, as renderPage does a page. The route is made with the test's store, as
// the app's are, so its loaders read into it.
export function renderRoutes(
  at: string,
  route: (store: AppStore) => RouteObject,
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
  const store = makeStore()
  const router = createMemoryRouter([route(store)], { initialEntries: [at] })
  const drawn = render(
    <Provider store={store}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </Provider>
  )
  return { ...drawn, asked }
}
