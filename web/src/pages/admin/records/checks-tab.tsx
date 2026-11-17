import { Notice } from "@/components/page"
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
import { plural } from "@/lib/format"
import { Figure, Figures } from "@/components/figures"
import type { CheckedRun, RunChecks } from "@/pages/admin/records/types"
import { withBase } from "@/lib/base"

// The server's word for a run whose files no longer match; the page says it in one word.
const failing = "does not check out"

// ChecksTab counts the newest finished runs by whether they still match what was recorded, and lists the ones to look at.
export function ChecksTab({ checks }: { checks: RunChecks }) {
  const rows = checks.Rows ?? []
  return (
    <>
      {checks.Failing > 0 && (
        <Notice>
          {checks.Failing} {plural(checks.Failing, "run")} no longer match what
          was recorded. Open one to see which file changed.
        </Notice>
      )}
      <Figures>
        <Figure value={checks.Verified} label="Verified" />
        <Figure value={checks.Unsigned} label="Not signed" />
        <Figure
          value={checks.Failing}
          label="Failed"
          bad={checks.Failing > 0}
        />
      </Figures>
      {rows.length > 0 ? (
        <NeedsALook rows={rows} />
      ) : (
        <p className="m-0 text-sm text-muted-foreground">
          {checks.Verified
            ? "Nothing needs a look."
            : "No run has finished yet."}
        </p>
      )}
    </>
  )
}

function NeedsALook({ rows }: { rows: CheckedRun[] }) {
  return (
    <>
      <h2 className="m-0 mb-3 text-sm font-semibold">Needs a look</h2>
      <Card className="@container gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-32 @max-md:hidden">When</TableHead>
              <TableHead>Run</TableHead>
              <TableHead className="@max-md:hidden">Who</TableHead>
              <TableHead>Result</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((run) => (
              <TableRow key={run.ID}>
                <TableCell className="text-muted-foreground @max-md:hidden">
                  {run.When}
                </TableCell>
                <TableCell className="w-full max-w-0 truncate">
                  <a
                    className="text-foreground no-underline underline-offset-4 hover:underline"
                    href={withBase(`${run.Href}#proof`)}
                  >
                    {run.Brief}
                  </a>
                </TableCell>
                <TableCell className="@max-md:hidden">{run.Who}</TableCell>
                <TableCell>
                  {run.State === failing ? (
                    <Badge variant="destructive">failed</Badge>
                  ) : (
                    <Badge variant="secondary">{run.State}</Badge>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </>
  )
}
