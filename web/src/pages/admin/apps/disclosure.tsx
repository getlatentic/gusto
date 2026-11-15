import type { ReactNode } from "react"
import { ChevronRightIcon } from "lucide-react"

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"

// Disclosure is words kept folded under a line that opens them, for what a reader needs once and not every time.
export function Disclosure({
  title,
  className,
  children,
}: {
  title: string
  className?: string
  children: ReactNode
}) {
  return (
    <Collapsible className={className}>
      <CollapsibleTrigger className="group/trigger flex cursor-pointer items-center gap-1 border-0 bg-transparent p-0 text-sm font-medium text-muted-foreground hover:text-foreground">
        <ChevronRightIcon
          className="size-4 transition-transform group-data-[panel-open]/trigger:rotate-90"
          aria-hidden="true"
        />
        {title}
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-2">{children}</CollapsibleContent>
    </Collapsible>
  )
}

// HowThisWorks is a folded paragraph that says how a page's part works.
export function HowThisWorks({
  title,
  className,
  children,
}: {
  title: string
  className?: string
  children: ReactNode
}) {
  return (
    <Disclosure title={title} className={className}>
      <p className="m-0 max-w-prose text-sm text-muted-foreground">
        {children}
      </p>
    </Disclosure>
  )
}
