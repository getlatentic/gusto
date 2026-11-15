import { AppLink } from "@/components/app-link"
import { showsOutcome, toneOf } from "@/domain/work"
import { cn } from "@/lib/utils"
import { CostText } from "@/pages/work/cost-text"
import { OriginLink } from "@/pages/work/origin-link"
import { StateBadge } from "@/pages/work/state-badge"
import type { Run } from "@/pages/work/types"

const dots = {
  ok: "bg-ok",
  bad: "bg-bad",
  warn: "bg-warn",
  plain: "bg-faint",
}

// RunLine is one run of Activity: what was asked, where, on what, how it ended, when, and what it cost.
export function RunLine({ run }: { run: Run }) {
  return (
    <li
      data-testid="run"
      data-id={run.ID}
      className="grid min-h-16 grid-cols-[0.5rem_minmax(0,1fr)_auto_11rem] items-center gap-4 px-5 py-3 hover:bg-muted/50 max-md:grid-cols-[0.5rem_minmax(0,1fr)_auto] max-md:gap-3 max-md:px-4"
    >
      <span
        className={cn("size-2 rounded-full", dots[toneOf(run.State)])}
        aria-hidden="true"
      />
      <div className="min-w-0">
        <AppLink
          href={`/jobs/${encodeURIComponent(run.ID)}`}
          title={run.Brief}
          data-testid="run-brief"
          className="block truncate text-base font-medium text-foreground no-underline hover:underline"
        >
          {run.Brief}
        </AppLink>
        <p className="m-0 mt-0.5 truncate text-xs text-muted-foreground">
          <OriginLink on={run.On} short className="text-muted-foreground" />
          {run.Brain && <span> · {run.Brain}</span>}
        </p>
        {run.Waiting && (
          <p className="m-0 text-xs text-muted-foreground">
            Asked by {run.AskedBy}. {run.Waiting}
          </p>
        )}
      </div>
      {showsOutcome(run.Outcome) ? (
        <StateBadge state={run.State} title={run.When}>
          {run.Outcome}
        </StateBadge>
      ) : (
        <span />
      )}
      <span className="text-right text-xs whitespace-nowrap text-muted-foreground tabular-nums max-md:hidden">
        <time title={run.When}>{run.Hour}</time>
        {run.Cost && (
          <>
            {" · "}
            <CostText cost={run.Cost} />
          </>
        )}
      </span>
    </li>
  )
}
