import {
  keepPreviousData,
  QueryClient,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"
import { useLocation } from "react-router"

// A page's data is the same view the server gave its template, as JSON at /app/api followed by the page's own address. The
// server keeps the query in the links it gives, so changing a filter is changing the address, and the address alone says what
// is shown.

export type Loaded<T> =
  | { state: "loading"; data?: T }
  | { state: "ready"; data: T }
  | { state: "failed"; status: number; message: string }

export class Failure extends Error {
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

export async function getJSON<T>(
  path: string,
  signal?: AbortSignal
): Promise<T> {
  const answer = await fetch(path, {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
    signal,
  })
  // A session that ended is sent to the sign-in page, which is a page and not JSON.
  if (answer.redirected && new URL(answer.url).pathname === "/login") {
    window.location.assign("/login")
    throw new Failure(401, "signed out")
  }
  if (!answer.ok) {
    throw new Failure(
      answer.status,
      (await answer.text()).trim() || answer.statusText
    )
  }
  return (await answer.json()) as T
}

// newQueryClient keeps each page's data by its address. A failure the server meant (a 4xx) is not asked again; anything else
// is, twice.
export function newQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: (count, err) =>
          !(err instanceof Failure && err.status < 500) && count < 2,
      },
    },
  })
}

// useData is the data at a path, fetched again when the path changes or the data is marked stale; what was shown stays on
// screen while the next one loads.
export function useData<T>(path: string): Loaded<T> {
  const query = useQuery({
    queryKey: [path],
    queryFn: ({ signal }) => getJSON<T>(path, signal),
    placeholderData: keepPreviousData,
  })
  if (query.isError) {
    const err = query.error
    return {
      state: "failed",
      status: err instanceof Failure ? err.status : 0,
      message: err.message,
    }
  }
  if (query.isSuccess && !query.isPlaceholderData) {
    return { state: "ready", data: query.data }
  }
  return { state: "loading", data: query.data }
}

// pagePath is where the data of the page the address names is.
function pagePath(where: { pathname: string; search: string }): string {
  return "/app/api" + where.pathname + where.search
}

// usePageData is the data of the page the address names.
export function usePageData<T>(): Loaded<T> {
  return useData<T>(pagePath(useLocation()))
}

// useRefreshPage marks the page's data stale, so it is fetched again, as after the page changed something the server keeps.
export function useRefreshPage(): () => Promise<void> {
  const client = useQueryClient()
  const path = pagePath(useLocation())
  return () => client.invalidateQueries({ queryKey: [path] })
}

// postForm sends a form to the server as the browser would, for a page that reads the answer itself, and gives the answer
// as JSON, or the server's reason when it refused.
export async function postForm<T>(path: string, form: FormData): Promise<T> {
  const fields = new URLSearchParams()
  for (const [key, value] of form) {
    if (typeof value === "string") fields.append(key, value)
  }
  const answer = await fetch(path, {
    method: "POST",
    credentials: "same-origin",
    headers: { Accept: "application/json" },
    body: fields,
  })
  if (!answer.ok) {
    throw new Failure(
      answer.status,
      (await answer.text()).trim() || answer.statusText
    )
  }
  return (await answer.json()) as T
}
