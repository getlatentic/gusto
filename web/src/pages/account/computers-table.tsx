import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import type { ApprovedComputer } from "@/pages/account/types"
import type { AccountForm } from "@/pages/account/use-account-form"

// ComputersTable is the computers a person approved: when, when each was last used, and the way to end one that is still live.
// On a narrow page when goes under the computer's name.
export function ComputersTable({
  computers,
  form,
}: {
  computers: ApprovedComputer[]
  form: AccountForm
}) {
  return (
    <Table className="@max-lg:[&_td]:px-2 @max-lg:[&_th]:px-2">
      <TableHeader>
        <TableRow>
          <TableHead>Computer</TableHead>
          <TableHead className="@max-lg:hidden">Approved</TableHead>
          <TableHead className="@max-lg:hidden">Last used</TableHead>
          <TableHead>
            <span className="sr-only">End</span>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {computers.map((computer) => (
          <TableRow key={computer.ID} data-testid="computer">
            <TableCell className="w-full max-w-0 font-medium">
              <span className="block truncate" title={computer.Name}>
                {computer.Name}
              </span>
              <span className="block text-xs font-normal whitespace-normal text-muted-foreground @lg:hidden">
                Approved {computer.Made}, last used{" "}
                {computer.LastUsed || "never"}
              </span>
            </TableCell>
            <TableCell className="text-muted-foreground @max-lg:hidden">
              {computer.Made}
            </TableCell>
            <TableCell className="text-muted-foreground @max-lg:hidden">
              {computer.LastUsed || "never"}
            </TableCell>
            <TableCell className="text-right whitespace-normal">
              {computer.Live ? (
                <form
                  method="post"
                  action={`/device/${encodeURIComponent(computer.ID)}/revoke`}
                  onSubmit={form.submit}
                  className="m-0"
                >
                  <Button
                    type="submit"
                    variant="destructive"
                    size="sm"
                    disabled={form.sending}
                  >
                    End
                  </Button>
                </form>
              ) : (
                <span className="text-muted-foreground">
                  ended {computer.Ended}
                </span>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
