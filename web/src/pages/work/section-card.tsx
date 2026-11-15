import type { ReactNode } from "react"

import { Card, CardContent } from "@/components/ui/card"

// SectionCard is one part of a work page: a section with its heading, and what it holds in a card.
export function SectionCard({
  title,
  id,
  testId,
  children,
}: {
  title?: ReactNode
  id?: string
  testId?: string
  children: ReactNode
}) {
  return (
    <section id={id} data-testid={testId} className="min-w-0 scroll-mt-20">
      <Card className="py-4">
        <CardContent className="grid min-w-0 gap-3 px-4">
          {title && <h2 className="m-0 text-sm font-semibold">{title}</h2>}
          {children}
        </CardContent>
      </Card>
    </section>
  )
}
