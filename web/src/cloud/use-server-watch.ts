import { useEffect } from "react"

import type { CloudPage } from "@/cloud/types"

// afterFailure is how long a page waits to ask again when the server could not be asked.
const afterFailure = 2000

// useServerWatch draws a page that waits on its organization's server again as soon as the server's state changes. The page
// asks gusto cloud, which answers once the state is not the one the page was drawn in.
export function useServerWatch(
  page: Pick<CloudPage, "Watch" | "Org"> | undefined
) {
  const seen = page?.Watch?.State
  const org = page?.Org?.Slug
  useEffect(() => {
    if (seen === undefined) return
    const stop = new AbortController()
    void watchServer(seen, org, stop.signal, () => window.location.reload())
    return () => stop.abort()
  }, [seen, org])
}

// watchServer asks gusto cloud, again and again, for the state of the organization's server, and calls changed once it is not
// seen. An answer that is not one, such as a sign-in page after the session ended, is a change too: drawing the page again shows it.
export async function watchServer(
  seen: string,
  org: string | undefined,
  signal: AbortSignal,
  changed: () => void,
  pause = afterFailure
) {
  const query = new URLSearchParams({ seen })
  if (org) query.set("org", org)
  while (!signal.aborted) {
    try {
      const answer = await fetch(`/org/server?${query}`, {
        signal,
        cache: "no-store",
        headers: { Accept: "application/json" },
      })
      if (answer.redirected) {
        changed()
        return
      }
      if (!answer.ok) throw new Error(answer.statusText)
      const { State } = (await answer.json()) as { State: string }
      if (State !== seen) {
        changed()
        return
      }
    } catch {
      if (signal.aborted) return
      await new Promise((resolve) => setTimeout(resolve, pause))
    }
  }
}
