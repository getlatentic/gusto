import type { ReactNode } from "react"

import { Label } from "@/components/ui/label"

// Field is one labelled control of a work page's form, with a quieter word after its name when it has one.
export function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <Label className="grid content-start gap-1.5 leading-normal">
      <span>
        {label}
        {hint && (
          <span className="font-normal text-muted-foreground"> {hint}</span>
        )}
      </span>
      {children}
    </Label>
  )
}
