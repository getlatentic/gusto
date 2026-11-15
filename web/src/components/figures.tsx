import type { ReactNode } from "react"

import { Card, CardContent } from "@/components/ui/card"
import { cn } from "@/lib/utils"

// Figures are a page's few counts side by side, as many to a row as fit; on a phone, two.
export function Figures({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <section
      className={cn(
        "mb-4 grid grid-cols-[repeat(auto-fit,minmax(min(8rem,100%),1fr))] gap-3",
        className
      )}
    >
      {children}
    </section>
  )
}

// Figure is one count: a word or two naming it, and the figure on one line. What qualifies the figure is its title; what
// measures it, such as a meter against a budget, goes under it.
export function Figure({
  value,
  label,
  detail,
  headline,
  bad,
  testId,
  children,
}: {
  value: ReactNode
  label: string
  detail?: string
  headline?: boolean
  bad?: boolean
  testId?: string
  children?: ReactNode
}) {
  return (
    <Card className="py-4" title={detail} data-testid={testId}>
      <CardContent className="grid gap-1 px-4">
        <span className="truncate text-xs font-medium text-muted-foreground">
          {label}
        </span>
        <span
          data-testid="figure-value"
          className={cn(
            "font-numeric text-xl font-semibold whitespace-nowrap tabular-nums sm:text-2xl",
            headline && "text-brand-ink",
            bad && "text-bad"
          )}
        >
          {value}
        </span>
        {detail && <span className="sr-only">{detail}</span>}
        {children}
      </CardContent>
    </Card>
  )
}
