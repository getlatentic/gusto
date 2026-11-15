import { AppLink } from "@/components/app-link"
import type { Origin } from "@/pages/work/types"

// OriginLink is where a run came from: a link to it, or, when there is nothing to go to, a short word whose title says why.
// short names only the agent, for a line that has the conversation's name elsewhere.
export function OriginLink({
  on,
  short = false,
  className,
}: {
  on: Origin
  short?: boolean
  className?: string
}) {
  if (!on.Link) {
    return (
      <span className="text-muted-foreground" title={on.Why}>
        {on.Named}
      </span>
    )
  }
  return (
    <AppLink
      href={on.Link}
      title={short ? on.Named : on.Why}
      className={className}
    >
      {short ? on.Project || on.Named : on.Named}
    </AppLink>
  )
}
