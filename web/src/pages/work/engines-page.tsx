import { ShieldCheckIcon } from "lucide-react"

import { AgentIcon } from "@/components/agent-icon"
import { Loading } from "@/components/controls"
import { PageBar, PageBody, PageSub } from "@/components/page"
import { Card } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { usePageData } from "@/lib/api"
import type { EnginesData, MeasuredCell } from "@/pages/work/types"

// EnginesPage is how each agent type did on the same small bug, on each model, in its own sealed box: the grid a person reads
// to choose an agent type.
export function EnginesPage() {
  const loaded = usePageData<EnginesData>()
  return (
    <>
      <PageBar
        title="How each agent type did"
        crumbs={[
          { label: "New agent", href: "/presets" },
          { label: "Your own agent", href: "/new/blank" },
        ]}
      />
      <PageBody>
        <PageSub>
          Each agent type was given the same small bug to fix on each model, in
          its own sealed box. A test decides whether it finished.
        </PageSub>
        <Loading loaded={loaded}>
          {(page) => <Grid measured={page.Measured} />}
        </Loading>
      </PageBody>
    </>
  )
}

function Grid({ measured }: { measured: EnginesData["Measured"] }) {
  const rows = measured.Rows ?? []
  if (rows.length === 0) {
    return (
      <Card className="px-6 py-12 text-center text-muted-foreground">
        <p className="m-0">Nothing has been measured on this server yet.</p>
      </Card>
    )
  }
  return (
    <>
      <p className="m-0 mb-5 flex items-baseline gap-2">
        <span className="font-numeric text-2xl font-semibold whitespace-nowrap text-brand-ink tabular-nums">
          {measured.Finished} of {measured.Tried}
        </span>
        <span className="text-sm text-muted-foreground">finished the fix</span>
      </p>
      <Card className="py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Agent type</TableHead>
              {(measured.Models ?? []).map((model) => (
                <TableHead key={model}>{model}</TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.Name} data-testid="engine-row">
                <TableCell className="font-semibold">
                  <span className="flex items-center gap-3">
                    {row.Mark && <AgentIcon src={row.Mark} />}
                    {row.Title}
                  </span>
                </TableCell>
                {row.Cells.map((cell, at) => (
                  <TableCell key={at}>
                    <Cell cell={cell} />
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
      <p className="m-0 mt-4 text-xs text-muted-foreground">
        Time includes starting the box. Cost is what the proxy counted.
      </p>
    </>
  )
}

function Cell({ cell }: { cell: MeasuredCell }) {
  if (!cell.Tried) return <span className="text-subtle">not tried</span>
  if (cell.Finished) {
    return (
      <>
        <span className="inline-flex items-center gap-1 font-medium text-ok">
          <ShieldCheckIcon aria-hidden="true" className="size-4" /> Finished
        </span>
        <span className="block text-xs text-subtle">
          {cell.Took}
          {cell.Cost && ` · ${cell.Cost}`}
        </span>
      </>
    )
  }
  return (
    <>
      <span className="font-medium text-bad">Did not finish</span>
      <span className="block text-xs text-subtle">{cell.Took}</span>
    </>
  )
}
