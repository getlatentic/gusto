import { StateBadge } from "@/pages/work/state-badge"
import type { TaskData } from "@/pages/work/types"

// TaskNotes is why a run ended early that the record says itself: a spending cap that refused it a call, or a full home.
export function TaskNotes({ page }: { page: TaskData }) {
  const capped = page.StoppedByCap
  const home = page.Home
  return (
    <>
      {capped && (
        <p className="m-0 mb-4 flex flex-wrap items-center gap-2 text-sm">
          <StateBadge state="failed">
            stopped by the {capped.Cap} cap
          </StateBadge>{" "}
          <span className="text-muted-foreground">{capped.Reason}</span>
        </p>
      )}
      {home && (
        <p
          role="alert"
          className="m-0 mb-4 max-w-4xl rounded-sm bg-bad-wash px-4 py-3 text-sm text-bad"
        >
          The home their agents keep is full: it takes {home.Used} of the{" "}
          {home.Limit} it may, so this turn was {home.Outcome || "ended"}. The
          person who asked can reset it on Connections.
        </p>
      )}
    </>
  )
}
