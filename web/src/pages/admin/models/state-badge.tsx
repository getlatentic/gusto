import type { ReactNode } from "react"

import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"

// StateBadge says whether a provider or model is in use: on, off and waiting, or not in use.
export function StateBadge({
  tone,
  children,
}: {
  tone: "ok" | "warn" | "bad"
  children: ReactNode
}) {
  return (
    <Badge
      variant={tone === "bad" ? "destructive" : "secondary"}
      className={cn(
        tone === "ok" && "bg-ok-wash text-ok",
        tone === "warn" && "bg-warn-wash text-warn"
      )}
    >
      {children}
    </Badge>
  )
}
