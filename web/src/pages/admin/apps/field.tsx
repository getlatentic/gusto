import type { ReactNode } from "react"

import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"

// Field is one labelled control of a form, with a quieter word after the label when it needs one.
export function Field({
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
      <Label htmlFor={id} className="flex-wrap">
        {label}
        {hint && (
          <span className="font-normal text-muted-foreground">{hint}</span>
        )}
      </Label>
      {children}
    </div>
  )
}

// Choices are the boxes a form ticks under one name, each with its words.
export function Choices({
  legend,
  name,
  choices,
}: {
  legend?: string
  name: string
  choices: { value: string; label: ReactNode; checked?: boolean }[]
}) {
  return (
    <fieldset className="m-0 grid gap-2 border-0 p-0">
      {legend && <legend className="mb-2 text-sm font-medium">{legend}</legend>}
      {choices.map((choice) => (
        <Label key={choice.value} className="items-start font-normal">
          <Checkbox
            name={name}
            value={choice.value}
            defaultChecked={choice.checked}
          />
          <span className="min-w-0 break-words">{choice.label}</span>
        </Label>
      ))}
    </fieldset>
  )
}
