import { useEffect, useEffectEvent, useState } from "react"

import { heard } from "@/domain/work"
import { withBase } from "@/lib/base"

// Watched is what a running task has said so far, and how it ended once it has.
export interface Watched {
  said: string
  ended: string
}

// useWatch follows a running task's words from the server's stream as it says them, and says how it ended. The stream is a
// subscription, opened while the task is live and closed when it ends or the page goes; onEnded is told when it ends. The page
// that uses it is drawn afresh for each task, so what it heard is always this task's.
export function useWatch(
  id: string,
  live: boolean,
  onEnded: () => void
): Watched {
  const [watched, setWatched] = useState<Watched>({ said: "", ended: "" })
  const ended = useEffectEvent(onEnded)
  useEffect(() => {
    if (!live) return
    const stream = new EventSource(withBase(`/watch/${encodeURIComponent(id)}`))
    // A stream starts the task's words from their beginning, so the first line it gives starts what is shown.
    let first = true
    stream.onmessage = (event: MessageEvent<string>) => {
      const from = first
      first = false
      setWatched((was) => ({
        ...was,
        said: heard(from ? "" : was.said, event.data),
      }))
    }
    stream.addEventListener("ended", (event) => {
      stream.close()
      setWatched((was) => ({
        ...was,
        ended: (event as MessageEvent<string>).data,
      }))
      ended()
    })
    return () => stream.close()
  }, [id, live])
  return watched
}
