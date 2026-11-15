import { ShieldCheckIcon } from "lucide-react"

import { AgentIcon } from "@/components/agent-icon"
import { AppLink } from "@/components/app-link"
import { Loading } from "@/components/controls"
import { PageBar, PageBody, PageSub } from "@/components/page"
import { Badge } from "@/components/ui/badge"
import { Card } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { sideResult } from "@/domain/work"
import { dataOf, usePageData } from "@/lib/api"
import { plural } from "@/lib/format"
import type { LineupData, Side } from "@/pages/work/types"

// While a run is still working, the page reads itself again this often.
const workingEvery = 4000

// LineupPage is one comparison: each agent's run of the same task side by side, and, once all are over, which stands clear.
export function LineupPage() {
  const first = usePageData<LineupData>()
  const loaded = usePageData<LineupData>({
    pollingInterval: dataOf(first)?.Running ? workingEvery : 0,
  })
  const brief = dataOf(loaded)?.Lineup.Brief
  return (
    <>
      <PageBar
        title={brief || "This task"}
        crumbs={[{ label: "Compare", href: "/compare" }]}
      />
      <PageBody>
        <Loading loaded={loaded}>{(page) => <Lineup page={page} />}</Loading>
      </PageBody>
    </>
  )
}

function Lineup({ page }: { page: LineupData }) {
  return (
    <>
      <p className="m-0 mb-1 text-lg font-semibold wrap-anywhere">
        {page.Lineup.Brief}
      </p>
      <PageSub>
        Same code, same model ({page.Lineup.Model}), same skills, same check.
        Only the agent differs.
      </PageSub>
      <p className="m-0 mb-3 flex items-baseline gap-2">
        <span
          className="font-numeric text-2xl font-semibold whitespace-nowrap text-brand-ink tabular-nums"
          data-testid="lineup-passed"
        >
          {page.Passed} of {page.Sides.length}
        </span>
        <span className="text-sm text-muted-foreground">
          {page.Running
            ? "passed so far, the rest still working"
            : "passed the tests"}
        </span>
      </p>
      {page.Reading && (
        <p className="m-0 mb-5 text-base" data-testid="lineup-reading">
          {page.Reading.Headline}
        </p>
      )}
      <Card className="py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Agent</TableHead>
              <TableHead>Result</TableHead>
              <TableHead>Time</TableHead>
              <TableHead>Cost</TableHead>
              <TableHead>Model calls</TableHead>
              <TableHead>Changed</TableHead>
              <TableHead>
                <span className="sr-only">Details</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {page.Sides.map((side) => (
              <SideRow key={side.Task} side={side} />
            ))}
          </TableBody>
        </Table>
      </Card>
      {page.Reading && (
        <p className="m-0 mt-4 max-w-[68ch] text-xs text-muted-foreground">
          Figures within 15% of each other, or under 5 seconds or a tenth of a
          cent apart, are called level: one run each is one sample, and a second
          run could reverse them. A model call is one turn of the agent&apos;s
          loop, counted at the proxy.
        </p>
      )}
      <p className="m-0 mt-2 text-xs text-muted-foreground">
        Each agent ran in its own sealed box from the same commit. Every run
        keeps its own details.
      </p>
    </>
  )
}

function SideRow({ side }: { side: Side }) {
  return (
    <TableRow data-testid="side">
      <TableCell className="font-semibold">
        <span className="flex items-center gap-3">
          {side.Mark && <AgentIcon src={side.Mark} />}
          <span>
            {side.Engine}
            <span className="block text-xs font-normal text-subtle">
              {side.Model}
            </span>
          </span>
        </span>
      </TableCell>
      <TableCell>
        <span className="flex flex-wrap items-center gap-2">
          <Result side={side} />
          {side.Fastest && <Badge variant="secondary">Fastest</Badge>}
          {side.Cheapest && <Badge variant="secondary">Cheapest</Badge>}
        </span>
      </TableCell>
      <TableCell className="tabular-nums">
        {side.Seconds > 0 && `${side.Seconds} s`}
      </TableCell>
      <TableCell className="tabular-nums">
        {side.Metered && side.Money}
      </TableCell>
      <TableCell className="tabular-nums">
        {side.Metered && side.Calls}
      </TableCell>
      <TableCell className="tabular-nums">
        {side.Files > 0 && (
          <>
            {side.Files} {plural(side.Files, "file")}{" "}
            <span className="text-ok">+{side.Added}</span>{" "}
            <span className="text-bad">−{side.Removed}</span>
          </>
        )}
      </TableCell>
      <TableCell>
        <AppLink href={`/jobs/${encodeURIComponent(side.Task)}`}>
          Details
        </AppLink>
      </TableCell>
    </TableRow>
  )
}

function Result({ side }: { side: Side }) {
  switch (sideResult(side)) {
    case "holding":
      return <span className="text-muted-foreground">{side.Holding}</span>
    case "working":
      return <span className="text-muted-foreground">Working…</span>
    case "passed":
      return (
        <span className="inline-flex items-center gap-1 font-medium text-ok">
          <ShieldCheckIcon aria-hidden="true" className="size-4" /> Tests pass
        </span>
      )
    case "failed":
      return <span className="font-medium text-bad">Tests fail</span>
    default:
      return (
        <span className="font-medium text-bad">Stopped before the tests</span>
      )
  }
}
