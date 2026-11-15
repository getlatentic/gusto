import type { ReactNode } from "react"

import { Card, CardContent } from "@/components/ui/card"
import { cn } from "@/lib/utils"

// PublicPanel is a page served before anybody is signed in: one panel in the middle of the window, and nothing around it that a
// session gives.
export function PublicPanel({
  className,
  children,
}: {
  className?: string
  children: ReactNode
}) {
  return (
    <main className="grid min-h-dvh place-items-center bg-background p-4">
      <title>gusto</title>
      <Card className={cn("w-full max-w-[22rem] shadow-md", className)}>
        <CardContent className="grid gap-6 px-8 py-2">{children}</CardContent>
      </Card>
    </main>
  )
}
