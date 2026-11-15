import { Card } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableRow } from "@/components/ui/table"
import { when } from "@/lib/format"
import { PostButton } from "./post-button"
import { TablePager } from "./table-pager"
import type { Pages, Step } from "./types"

// Steps are the earlier states of the registry, each with what was changed after it, and the way back to it.
export function Steps({ steps, pages }: { steps: Step[]; pages?: Pages }) {
  return (
    <section className="grid gap-3">
      <div className="grid gap-1">
        <h2 className="m-0 text-sm font-semibold">
          Go back to an earlier version
        </h2>
        <p className="m-0 max-w-prose text-xs text-muted-foreground">
          Each earlier state of the registry is kept, and going back to one is a
          change of its own, so it can be gone back from. What was added is off
          until you turn it on. A key kept here is forgotten when its provider
          is retired, so a provider that comes back that way has no key until
          you give it one.
        </p>
      </div>
      <Card className="@container gap-0 overflow-hidden py-0">
        <Table>
          <TableBody>
            {steps.map((step) => (
              <TableRow key={step.Version}>
                <TableCell className="whitespace-nowrap">
                  Version {step.Version}
                </TableCell>
                <TableCell className="whitespace-nowrap text-muted-foreground @max-lg:hidden">
                  {when(step.At)}
                </TableCell>
                <TableCell className="w-full max-w-0 whitespace-normal">
                  then:{" "}
                  {step.Then?.length
                    ? step.Then.join("; ")
                    : "no change you can see"}
                </TableCell>
                <TableCell className="text-right">
                  <PostButton
                    action="/admin/models/restore"
                    fields={{ version: String(step.Version) }}
                    variant="outline"
                    size="sm"
                  >
                    Go back to it
                  </PostButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <TablePager pages={pages} />
      </Card>
    </section>
  )
}
