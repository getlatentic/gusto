import type { LoaderFunction } from "react-router"

import { pagePath } from "@/lib/api"
import type { AppStore } from "@/store"
import { api } from "@/store/api"

// readAhead starts reading a page's data as the router moves to it, while the page's code is still being fetched, so the one
// does not wait for the other. The page reads the same address with useData, and RTK Query keeps the data. The loader returns
// nothing and does not wait, so the move is never held up by the read.
export function readAhead(
  store: AppStore,
  pathOf: (url: URL) => string = pagePath
): LoaderFunction {
  return ({ request }) => {
    store.dispatch(api.util.prefetch("page", pathOf(new URL(request.url)), {}))
    return null
  }
}
