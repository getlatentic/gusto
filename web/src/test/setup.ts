import { afterEach, vi } from "vitest"
import { cleanup, configure } from "@testing-library/react"

// jsdom has none of these, and Base UI's popups ask for them.
class Observer {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= Observer as unknown as typeof ResizeObserver
globalThis.IntersectionObserver ??=
  Observer as unknown as typeof IntersectionObserver
window.matchMedia ??= ((query: string) => ({
  matches: false,
  media: query,
  addEventListener() {},
  removeEventListener() {},
})) as unknown as typeof window.matchMedia

// Each page's code is fetched when its route is first opened, as in the app, which takes longer than the default second while
// the whole suite runs at once.
configure({ asyncUtilTimeout: 5_000 })

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})
