import type { ReactNode } from "react"

import { Card } from "@/components/ui/card"

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
