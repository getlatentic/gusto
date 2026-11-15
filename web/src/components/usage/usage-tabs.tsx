import { LinkTabs } from "@/components/controls"
import type { UsagePlace } from "@/components/usage/usage-place"

// The two views of the usage: the month's summary, and the conversations one by one.
export function UsageTabs({
  place,
  current,
}: {
  place: UsagePlace
  current: "summary" | "tasks"
}) {
  return (
    <LinkTabs
      label="Usage views"
      current={current}
      tabs={[
        { key: "summary", label: "Summary", href: place.summary },
        { key: "tasks", label: "Conversations", href: place.conversations },
      ]}
    />
  )
}
