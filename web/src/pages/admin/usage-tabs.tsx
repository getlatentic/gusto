import { LinkTabs } from "@/components/controls"

// The two views of the usage: the month's summary, and the conversations one by one.
export function UsageTabsFor({ current }: { current: "summary" | "tasks" }) {
  return (
    <LinkTabs
      label="Usage views"
      current={current}
      tabs={[
        { key: "summary", label: "Summary", href: "/admin/spending" },
        { key: "tasks", label: "Conversations", href: "/admin/usage" },
      ]}
    />
  )
}
