import { useRef, type ReactNode } from "react"
import { EllipsisIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

// Section is one titled part of an organization's page, with the action that adds to it beside its title.
export function Section({
  title,
  id,
  action,
  children,
}: {
  title: string
  id?: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <Card className="py-5" id={id}>
      <CardContent className="grid gap-3 px-5">
        <div className="flex min-h-8 items-center justify-between gap-3">
          <h2 className="m-0 text-sm font-semibold">{title}</h2>
          {action}
        </div>
        {children}
      </CardContent>
    </Card>
  )
}

// A RowAction is one thing a row's menu does: send a form of hidden fields, or open what onSelect opens.
export type RowAction = {
  label: string
  destructive?: boolean
} & (
  { action: string; fields: Record<string, string> } | { onSelect: () => void }
)

// RowMenu is what can be done to one row, behind ⋯.
export function RowMenu({
  label,
  actions,
}: {
  label: string
  actions: RowAction[]
}) {
  if (actions.length === 0) return null
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={`More for ${label}`}
          />
        }
      >
        <EllipsisIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        {actions.map((action) =>
          "onSelect" in action ? (
            <DropdownMenuItem key={action.label} onClick={action.onSelect}>
              {action.label}
            </DropdownMenuItem>
          ) : (
            <PostItem key={action.label} action={action} />
          )
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// PostItem is a menu item that sends its own form of hidden fields.
function PostItem({
  action,
}: {
  action: Extract<RowAction, { action: string }>
}) {
  const form = useRef<HTMLFormElement>(null)
  return (
    <>
      <form ref={form} method="post" action={action.action} hidden>
        {Object.entries(action.fields).map(([name, value]) => (
          <input key={name} type="hidden" name={name} value={value} />
        ))}
      </form>
      <DropdownMenuItem
        variant={action.destructive ? "destructive" : "default"}
        onClick={() => form.current?.requestSubmit()}
      >
        {action.label}
      </DropdownMenuItem>
    </>
  )
}

// Row is one line of a section's list: what it is, a word about it, and its menu.
export function Row({
  children,
  note,
  menu,
}: {
  children: ReactNode
  note?: ReactNode
  menu?: ReactNode
}) {
  return (
    <li className="flex min-h-11 items-center gap-3 border-b py-1.5 last:border-b-0">
      <span className="min-w-0 flex-1 wrap-anywhere">{children}</span>
      {note && <span className="text-muted-foreground">{note}</span>}
      <span className="flex w-7 justify-end">{menu}</span>
    </li>
  )
}
