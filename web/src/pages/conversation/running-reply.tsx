import { useEffect, useEffectEvent, useState } from "react"
import { CheckIcon } from "lucide-react"

import { dataOf, useData, useRefreshAll } from "@/lib/api"
import { cn } from "@/lib/utils"
import type { ConversationData } from "@/pages/conversation/types"

interface LiveItem {
  kind: "said" | "step"
  text?: string
  what?: string
  detail?: string
}

interface LiveTurn {
  items: LiveItem[] | null
  steps: number
  cut?: boolean
  holding?: string
}

// elapsed is how long something has been going, as a clock reads it.
function elapsed(seconds: number): string {
  const minutes = Math.floor(seconds / 60)
  return minutes
    ? `${minutes}m ${String(seconds % 60).padStart(2, "0")}s`
    : `${seconds}s`
}

// useEnded hears from the server when a running turn ends, which is when the conversation is fetched again with its reply.
function useEnded(task: string, onEnded: () => void) {
  const ended = useEffectEvent(onEnded)
  useEffect(() => {
    const watch = new EventSource(`/watch/${encodeURIComponent(task)}`)
    watch.addEventListener("ended", () => {
      watch.close()
      ended()
    })
    return () => watch.close()
  }, [task])
}

// RunningReply is the reply of a conversation that is still being written: what it is doing now, how long it has taken, and
// everything it has said and done so far, as it came.
export function RunningReply({ page }: { page: ConversationData }) {
  const refreshAll = useRefreshAll()
  const [started] = useState(() => Date.now())
  const [now, setNow] = useState(started)
  const live = dataOf(
    useData<LiveTurn>(`${page.Path}/live`, { pollingInterval: 1_500 })
  )
  useEffect(() => {
    const tick = window.setInterval(() => setNow(Date.now()), 1_000)
    return () => window.clearInterval(tick)
  }, [])
  useEnded(page.Latest, refreshAll)
  const items = live?.items ?? []
  const steps = items.filter((item) => item.kind === "step")
  const latest = steps[steps.length - 1]
  const doing = live?.holding || page.Holding || latest?.what || "Working"
  return (
    <section aria-label="The reply so far" className="mt-5 ml-10.5 grid gap-3">
      <p className="m-0 flex items-center gap-3 text-sm font-medium">
        <span
          className="size-3.5 flex-none animate-spin rounded-full border-2 border-input border-t-foreground"
          aria-hidden="true"
        />
        <span role="status">{doing}</span>
        <span className="ml-auto font-normal text-subtle tabular-nums">
          {elapsed(Math.floor((now - started) / 1000))}
        </span>
      </p>
      {items.length > 0 && (
        <ol
          aria-label="What it has done so far"
          className="m-0 grid list-none gap-2 p-0 pl-2"
        >
          {live?.cut && (
            <li className="text-xs text-subtle">
              The start of this turn is left out.
            </li>
          )}
          {items.map((item, index) =>
            item.kind === "said" ? (
              <li key={index} className="text-sm whitespace-pre-wrap">
                {item.text}
              </li>
            ) : (
              <li
                key={index}
                className={cn(
                  "flex items-start gap-2 text-xs",
                  item === latest ? "text-foreground" : "text-muted-foreground"
                )}
              >
                <CheckIcon
                  aria-hidden="true"
                  className={cn(
                    "mt-0.5 size-3.5 flex-none",
                    item === latest && "invisible"
                  )}
                />
                <span className="min-w-0">
                  <span className="mr-2 font-semibold">{item.what}</span>
                  {item.detail && (
                    <span className="font-mono wrap-anywhere">
                      {item.detail}
                    </span>
                  )}
                </span>
              </li>
            )
          )}
        </ol>
      )}
    </section>
  )
}
