import { useEffect, useState } from "react"
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

// useData fetches a path, and fetches again when it changes; what was shown stays on screen while the next one loads.
export function useData<T>(path: string): Loaded<T> {
  const [answer, setAnswer] = useState<{ path: string; loaded: Loaded<T> }>()
  useEffect(() => {
    const stop = new AbortController()
    getJSON<T>(path, stop.signal).then(
      (data) => setAnswer({ path, loaded: { state: "ready", data } }),
      (err: unknown) => {
        if (stop.signal.aborted) return
        const status = err instanceof Failure ? err.status : 0
        setAnswer({
          path,
          loaded: {
            state: "failed",
            status,
            message: err instanceof Error ? err.message : String(err),
          },
        })
      }
    )
    return () => stop.abort()
  }, [path])
  if (answer?.path === path) return answer.loaded
  return {
    state: "loading",
    data: answer?.loaded.state === "ready" ? answer.loaded.data : undefined,
  }
}

// usePageData is the data of the page the address names.
export function usePageData<T>(): Loaded<T> {
  const where = useLocation()
  return useData<T>("/app/api" + where.pathname + where.search)
}
