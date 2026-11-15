import type { ReactElement, ReactNode } from "react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

// FormDialog is one form of the page in a dialog: what it is for, its fields, and the button that posts it to action. It opens
// from its own trigger, or from whoever holds open.
export function FormDialog({
  trigger,
  label,
  open,
  onOpenChange,
  title,
  words,
  action,
  submit,
  className,
  children,
}: {
  trigger?: ReactElement
  label?: ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
  title: string
  words: ReactNode
  action: string
  submit: string
  className?: string
  children: ReactNode
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {trigger && <DialogTrigger render={trigger}>{label}</DialogTrigger>}
      <DialogContent
        className={cn("max-h-[calc(100dvh-4rem)] overflow-y-auto", className)}
      >
        <form method="post" action={action} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{words}</DialogDescription>
          </DialogHeader>
          {children}
          <DialogFooter>
            <DialogClose render={<Button variant="outline" type="button" />}>
              Cancel
            </DialogClose>
            <Button type="submit">{submit}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// Field is one field of a form with its label above it.
export function Field({
  id,
  label,
  children,
}: {
  id: string
  label: ReactNode
  children: ReactNode
}) {
  return (
    <div className="grid min-w-0 gap-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
    </div>
  )
}
