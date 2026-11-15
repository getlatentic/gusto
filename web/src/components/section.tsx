import type { ReactNode } from "react"

import { Card, CardContent } from "@/components/ui/card"

// Section is one titled part of a page, its title above a card, with how many it holds beside the title when that is said.
export function Section({
  title,
  count,
  children,
}: {
  title: string
  count?: number
  children: ReactNode
}) {
  return (
    <section className="grid min-w-0 gap-2">
      <h2 className="m-0 text-sm font-semibold">
        {title}
        {count !== undefined && (
          <span className="font-normal text-muted-foreground"> {count}</span>
        )}
      </h2>
      <Card>
        <CardContent>{children}</CardContent>
      </Card>
    </section>
  )
}
