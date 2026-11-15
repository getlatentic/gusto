import type { ReactNode } from "react"

import { Badge } from "@/components/ui/badge"
import { toneOf } from "@/domain/work"
import { cn } from "@/lib/utils"

// StateBadge is a short word for how something stands, in the colour of what the state means.
export function StateBadge({
  state,
  children,
  title,
  testId,
}: {
  state: string
  children: ReactNode
  title?: string
  testId?: string
}) {
  const tone = toneOf(state)
  return (
    <Badge
      variant={tone === "bad" ? "destructive" : "secondary"}
      title={title}
      data-testid={testId}
      className={cn(
        tone === "ok" && "bg-ok-wash text-ok",
        tone === "warn" && "bg-warn-wash text-warn"
      )}
    >
      {children}
    </Badge>
  )
}
