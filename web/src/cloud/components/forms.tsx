import { useId, useState, type ComponentProps, type ReactNode } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

// Field is one labelled box of a form.
export function Field({
  label,
  className,
  ...input
}: { label: ReactNode } & ComponentProps<typeof Input>) {
  const id = useId()
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={id} className="text-muted-foreground">
        {label}
      </Label>
      <Input id={id} className={cn("h-11", className)} {...input} />
    </div>
  )
}

// Submit is a form's one way to send it, as wide as the form.
export function Submit({ className, ...props }: ComponentProps<typeof Button>) {
  return <Button className={cn("h-11", className)} type="submit" {...props} />
}

// OnceForm is a form that does something once, such as joining by an invitation: a press while it is on its way does nothing,
// and its button says it is working.
export function OnceForm({
  button,
  busy,
  children,
  ...form
}: {
  button: string
  busy: string
  children?: ReactNode
} & ComponentProps<"form">) {
  const [sent, setSent] = useState(false)
  return (
    <form
      className="grid gap-3"
      method="post"
      {...form}
      onSubmit={(event) => {
        if (sent) event.preventDefault()
        else setSent(true)
      }}
    >
      {children}
      <Submit aria-disabled={sent || undefined} data-testid="confirm">
        {sent ? busy : button}
      </Submit>
    </form>
  )
}
