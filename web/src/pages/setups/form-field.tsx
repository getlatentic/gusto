import type { ReactNode } from "react"

import { Label } from "@/components/ui/label"

// FormField is one field of a form with its label above it, and a quieter word after the label when the field needs one.
export function FormField({
  id,
  label,
  hint,
  children,
}: {
  id: string
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>
        {label}
        {hint && (
          <span className="font-normal text-muted-foreground">{hint}</span>
        )}
      </Label>
      {children}
    </div>
  )
}
