import type { ReactNode } from "react"
import { ChevronRightIcon } from "lucide-react"

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { cn } from "@/lib/utils"

// Disclosure is a part of a page folded under a line that opens it, as a details element is. A heading disclosure is a
// section's heading that opens it.
export function Disclosure({
  summary,
  open,
  heading = false,
  className,
  testId,
  children,
}: {
  summary: ReactNode
  open?: boolean
  heading?: boolean
  className?: string
  testId?: string
  children: ReactNode
}) {
  const trigger = (
    <CollapsibleTrigger
      className={cn(
        "flex cursor-pointer items-center gap-2 border-0 bg-transparent p-0 text-left text-sm font-medium text-muted-foreground hover:text-foreground data-[panel-open]:[&>svg]:rotate-90",
        heading && "font-semibold text-foreground"
      )}
    >
      <ChevronRightIcon
        aria-hidden="true"
        className="size-4 flex-none transition-transform"
      />
      {summary}
    </CollapsibleTrigger>
  )
  return (
    <Collapsible defaultOpen={open} className={className} data-testid={testId}>
      {heading ? <h2 className="m-0 text-sm">{trigger}</h2> : trigger}
      <CollapsibleContent className="mt-3">{children}</CollapsibleContent>
    </Collapsible>
  )
}
