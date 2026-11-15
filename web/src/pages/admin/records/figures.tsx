import type { ReactNode } from "react"

import { Card, CardContent } from "@/components/ui/card"
import { cn } from "@/lib/utils"

// Figures are a tab's few counts side by side, as many to a row as fit.
export function Figures({ children }: { children: ReactNode }) {
  return (
    <section className="mb-4 grid grid-cols-[repeat(auto-fit,minmax(min(9rem,100%),1fr))] gap-3">
      {children}
    </section>
  )
}

export function Figure({
  value,
  label,
  bad,
}: {
  value: ReactNode
  label: ReactNode
  bad?: boolean
}) {
  return (
    <Card className="py-4">
      <CardContent className="flex flex-col-reverse justify-between gap-2 px-4">
        <span className="text-xs font-medium text-muted-foreground">
          {label}
        </span>
        <span
          className={cn(
            "font-numeric text-2xl font-semibold tabular-nums",
            bad && "text-bad"
          )}
        >
          {value}
        </span>
      </CardContent>
    </Card>
  )
}

// Section is one titled part of a tab, its content on a card.
export function Section({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section className="grid gap-2">
      <h2 className="m-0 px-1 text-sm font-semibold">{title}</h2>
      <Card className="@container gap-3 px-4 py-4 text-sm">{children}</Card>
    </section>
  )
}
