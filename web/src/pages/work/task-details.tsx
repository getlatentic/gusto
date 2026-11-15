import type { ReactNode } from "react"

import { AppLink } from "@/components/app-link"
import { CostText } from "@/pages/work/cost-text"
import { OriginLink } from "@/pages/work/origin-link"
import { StateBadge } from "@/pages/work/state-badge"
import type { TaskData } from "@/pages/work/types"

const plainLink = "text-inherit no-underline hover:underline"

// TaskSummary is how the run stands, how long it took and what it cost. ended is how a run the page watched ended, said at
// once, before the page is read again.
export function TaskSummary({
  page,
  ended,
}: {
  page: TaskData
  ended: string
}) {
  return (
    <p
      className="m-0 mb-4 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm"
      aria-label="Summary"
    >
      <StateBadge state={ended || page.Job.State} testId="task-state">
        {ended || page.Outcome}
      </StateBadge>
      <span className="text-muted-foreground">
        {page.Took || "—"}
        {page.Cost && (
          <>
            {" · "}
            <CostText cost={page.Cost} />
          </>
        )}
      </span>
    </p>
  )
}

function Row({ term, children }: { term: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 items-baseline gap-4 border-b py-2.5 last:border-b-0">
      <dt className="w-32 flex-none text-sm text-muted-foreground">{term}</dt>
      <dd className="m-0 min-w-0 flex-1 truncate text-sm font-semibold">
        {children}
      </dd>
    </div>
  )
}

// TaskDetails is what the run spent and ran on: its tokens, its check, its agent or where it came from, its build and model,
// and whether its record checks out.
export function TaskDetails({ page }: { page: TaskData }) {
  const verdict = page.Check?.Verdict
  return (
    <dl className="m-0 mb-5 max-w-4xl" aria-label="Details">
      {page.Calls && (
        <Row term="Tokens">
          {page.Calls.InputTokens} in · {page.Calls.OutputTokens} out
        </Row>
      )}
      {page.Check && (
        <Row term="Check">
          {verdict ? (
            <StateBadge state={verdict.Passed ? "passed" : "failed"}>
              {verdict.Passed ? "passed" : "failed"}
            </StateBadge>
          ) : (
            "no answer"
          )}
        </Row>
      )}
      <Row term="Agent">
        {page.Agent ? (
          <AppLink
            className={plainLink}
            href={`/instances/${encodeURIComponent(page.Agent.ID)}`}
          >
            {page.Agent.Name}
          </AppLink>
        ) : (
          page.Job.Harness
        )}
      </Row>
      {page.Build && <Row term="Build">{page.Build}</Row>}
      {!page.Agent && (
        <Row term="Came from">
          <OriginLink on={page.On} className={plainLink} />
        </Row>
      )}
      <Row term="Model">{page.Agent?.Brain || page.Brain || "—"}</Row>
      {page.Proof && !page.Proof.Holds && (
        <Row term="Integrity">
          <a className="text-bad no-underline hover:underline" href="#proof">
            does not check out
          </a>
        </Row>
      )}
    </dl>
  )
}
