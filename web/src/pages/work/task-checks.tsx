import { CheckIcon } from "lucide-react"

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { plural } from "@/lib/format"
import { CutNote } from "@/pages/work/cut-note"
import { Disclosure } from "@/pages/work/disclosure"
import { SectionCard } from "@/pages/work/section-card"
import { StateBadge } from "@/pages/work/state-badge"
import type { Step, TaskData } from "@/pages/work/types"

// TaskSteps is what the run did, one plain line a step, open to read.
export function TaskSteps({ steps }: { steps: Step[] }) {
  if (steps.length === 0) return null
  return (
    <SectionCard title="What it did" testId="task-steps">
      <Disclosure
        open
        summary={`${steps.length} ${plural(steps.length, "step")}`}
      >
        <ol className="m-0 grid list-none gap-3 p-0 text-sm">
          {steps.map((step, at) => (
            <li
              key={at}
              className="flex min-w-0 items-start gap-3 wrap-anywhere"
            >
              <span className="mt-0.5 grid size-5 flex-none place-items-center rounded-full bg-ok-wash text-ok">
                <CheckIcon aria-hidden="true" className="size-3" />
              </span>
              <span className="min-w-0">
                <span className="mr-1 font-semibold">{step.What}</span>
                {step.Detail && (
                  <span className="block text-xs text-muted-foreground">
                    {step.Detail}
                  </span>
                )}
              </span>
            </li>
          ))}
        </ol>
      </Disclosure>
    </SectionCard>
  )
}

// TaskNetwork is each site outside the model the agent asked for, which was refused, and how often it tried.
export function TaskNetwork({ page }: { page: TaskData }) {
  const calls = page.Calls
  if (!calls || calls.Denied === 0) return null
  return (
    <SectionCard title="Network" testId="task-network">
      <p className="m-0 text-xs text-muted-foreground">
        The agent asked for {calls.Denied}{" "}
        {calls.Denied === 1 ? "address" : "addresses"} outside the model, and
        each was refused.
      </p>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Site</TableHead>
            <TableHead className="w-20 text-right">Times</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(calls.Sites ?? []).map((site) => (
            <TableRow key={site.Host}>
              <TableCell>
                <code className="text-xs">{site.Host}</code>
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {site.Times}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </SectionCard>
  )
}

const output =
  "m-0 max-h-96 overflow-auto rounded-sm bg-muted p-4 whitespace-pre-wrap"

// TaskChecks is the check's answer: its verdict beside the command and its exit, and what it printed, drawn open when it did
// not pass, since that is what the reader came for.
export function TaskChecks({ page }: { page: TaskData }) {
  const check = page.Check
  if (!check) return null
  const verdict = check.Verdict
  const log = check.Log
  return (
    <SectionCard title="The check" testId="task-check">
      {verdict ? (
        <p className="m-0 flex flex-wrap items-center gap-3 text-sm">
          <StateBadge state={verdict.Passed ? "passed" : "failed"}>
            {verdict.Passed ? "passed" : "failed"}
          </StateBadge>
          <code className="text-xs wrap-anywhere">{check.Command}</code>{" "}
          <span className="text-muted-foreground">exited {verdict.Code}</span>
        </p>
      ) : (
        <p className="m-0 text-muted-foreground">
          The run stopped before its check answered.
        </p>
      )}
      {log &&
        (check.Explains ? (
          <>
            <h3 className="m-0 mt-2 text-sm">What the check said</h3>
            <pre className={output}>{log.Text}</pre>
            <CutNote cut={log} />
          </>
        ) : (
          <Disclosure summary={`What the check printed (${log.Whole})`}>
            <pre className={output}>{log.Text}</pre>
            <CutNote cut={log} />
          </Disclosure>
        ))}
    </SectionCard>
  )
}
