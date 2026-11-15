import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { plural } from "@/lib/format"
import { Figure, Figures, Section } from "@/pages/admin/records/figures"
import { Holds } from "@/pages/admin/records/holds"
import type { Keeping, PolicyRow } from "@/pages/admin/records/types"

// KeepingTab is what the tasks take on disk, the policy that removes finished work, the next sweep, and what is held from it.
export function KeepingTab({ keeping }: { keeping: Keeping }) {
  return (
    <>
      <Figures>
        <Figure
          value={keeping.Used}
          label={`used by ${keeping.Jobs} ${plural(keeping.Jobs, "task")}`}
        />
        <Figure
          value={keeping.WorkingFolders}
          label="of it in working folders"
        />
        <Figure
          value={keeping.OldestAt || "none"}
          label={
            keeping.Oldest ? (
              <>
                oldest task kept,{" "}
                <a className="underline" href={`/jobs/${keeping.Oldest}`}>
                  {keeping.Oldest}
                </a>
              </>
            ) : (
              "no task has finished"
            )
          }
        />
      </Figures>
      <div className="grid gap-9">
        <Section title="Policy">
          <Policy keeping={keeping} />
        </Section>
        <Section title="Next sweep">
          <NextSweep keeping={keeping} />
        </Section>
        <Section title="Holds">
          <Holds tasks={keeping.HeldTasks} people={keeping.HeldPeople} />
        </Section>
      </div>
    </>
  )
}

const days = (count: number) =>
  count ? `${count} days` : <span className="text-muted-foreground">kept</span>

function Policy({ keeping }: { keeping: Keeping }) {
  if (!keeping.Removes) {
    return (
      <p className="m-0">
        The policy removes nothing. Set <code>workspaceDays</code> and{" "}
        <code>recordDays</code> by state in <code>policy.json</code> in{" "}
        <code>GUSTO_RETENTION</code>.
      </p>
    )
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="whitespace-normal">A task that</TableHead>
          <TableHead className="whitespace-normal">
            loses its working folder after
          </TableHead>
          <TableHead className="whitespace-normal">
            loses its record after
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {(keeping.Policy ?? []).map((row: PolicyRow) => (
          <TableRow key={row.State}>
            <TableCell>{row.State}</TableCell>
            <TableCell>{days(row.WorkingFolder)}</TableCell>
            <TableCell>{days(row.Record)}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

// NextSweep is what a sweep now would remove, the way to make it, and what the last one did.
function NextSweep({ keeping }: { keeping: Keeping }) {
  const upcoming = keeping.Upcoming ?? []
  return (
    <>
      {upcoming.length > 0 ? (
        <>
          <p className="m-0">
            A sweep now would remove {upcoming.length}{" "}
            {plural(upcoming.length, "item")}, freeing {keeping.UpcomingFreed}.
          </p>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Task</TableHead>
                <TableHead className="@max-md:hidden">Removes</TableHead>
                <TableHead className="@max-md:hidden">Ended</TableHead>
                <TableHead className="text-right">Frees</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {upcoming.map((action) => (
                <TableRow key={`${action.job} ${action.kind}`}>
                  <TableCell className="w-full max-w-0 truncate">
                    <a className="underline" href={`/jobs/${action.job}`}>
                      {action.brief}
                    </a>
                  </TableCell>
                  <TableCell className="@max-md:hidden">
                    {action.kind}
                  </TableCell>
                  <TableCell className="@max-md:hidden">
                    {action.days} days ago
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {action.bytes} bytes
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </>
      ) : (
        <p className="m-0 text-muted-foreground">Nothing is due.</p>
      )}
      <form method="post" action="/admin/retention/sweep" className="m-0">
        <Button size="sm" variant="outline" type="submit">
          Sweep now
        </Button>
      </form>
      {keeping.Last && (
        <p className="m-0 text-muted-foreground">
          The last sweep was {keeping.LastAt}:{" "}
          {keeping.Last.actions?.length ?? 0} removed, {keeping.LastFreed}{" "}
          freed.
        </p>
      )}
    </>
  )
}
