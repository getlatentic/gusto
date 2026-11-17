import type { ReactNode } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import type { Hold } from "@/pages/admin/records/types"
import { withBase } from "@/lib/base"

type Held = Record<string, Hold> | null

// sorted is a map's entries by key, the order the server's templates range over a map in.
const sorted = (held: Held) =>
  Object.entries(held ?? {}).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))

// Holds are the tasks and the people whose work no sweep removes, each with its release, and the forms that hold more.
export function Holds({ tasks, people }: { tasks: Held; people: Held }) {
  const heldTasks = sorted(tasks)
  const heldPeople = sorted(people)
  return (
    <>
      {heldTasks.length + heldPeople.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>What</TableHead>
              <TableHead className="@max-md:hidden">Held by</TableHead>
              <TableHead className="@max-md:hidden">Why</TableHead>
              <TableHead>
                <span className="sr-only">Release</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {heldTasks.map(([id, hold]) => (
              <HoldLine
                key={`task ${id}`}
                hold={hold}
                release="/admin/retention/jobs/release"
                field="job"
                value={id}
              >
                task{" "}
                <a className="underline" href={withBase(`/jobs/${id}`)}>
                  {id}
                </a>
              </HoldLine>
            ))}
            {heldPeople.map(([who, hold]) => (
              <HoldLine
                key={`person ${who}`}
                hold={hold}
                release="/admin/retention/holds/release"
                field="who"
                value={who}
              >
                everything asked for by {who}
              </HoldLine>
            ))}
          </TableBody>
        </Table>
      )}
      <HoldForm
        action="/admin/retention/jobs/hold"
        id="hold-task"
        label="Task"
        name="job"
        placeholder="the task's id, as in its address"
        submit="Keep the task"
      />
      <HoldForm
        action="/admin/retention/holds"
        id="hold-person"
        label="Person"
        name="who"
        placeholder="amy@acme.test"
        submit="Keep their work"
      />
    </>
  )
}

function HoldLine({
  hold,
  release,
  field,
  value,
  children,
}: {
  hold: Hold
  release: string
  field: string
  value: string
  children: ReactNode
}) {
  return (
    <TableRow>
      <TableCell className="w-full max-w-0 break-words whitespace-normal">
        {children}
      </TableCell>
      <TableCell className="@max-md:hidden">{hold.by}</TableCell>
      <TableCell className="whitespace-normal @max-md:hidden">
        {hold.why}
      </TableCell>
      <TableCell className="text-right">
        <form method="post" action={withBase(release)} className="m-0">
          <input type="hidden" name={field} value={value} />
          <Button size="sm" variant="outline" type="submit">
            Release
          </Button>
        </form>
      </TableCell>
    </TableRow>
  )
}

// HoldForm holds one more thing: what it is, and why, for the record.
function HoldForm({
  action,
  id,
  label,
  name,
  placeholder,
  submit,
}: {
  action: string
  id: string
  label: string
  name: string
  placeholder: string
  submit: string
}) {
  return (
    <form
      method="post"
      action={withBase(action)}
      className="m-0 flex flex-wrap items-end gap-3"
    >
      <div className="grid min-w-40 flex-1 gap-2">
        <Label htmlFor={id}>{label}</Label>
        <Input id={id} name={name} placeholder={placeholder} required />
      </div>
      <div className="grid min-w-40 flex-1 gap-2">
        <Label htmlFor={`${id}-why`}>Why</Label>
        <Input id={`${id}-why`} name="why" placeholder="for the record" />
      </div>
      <Button size="sm" variant="outline" type="submit">
        {submit}
      </Button>
    </form>
  )
}
