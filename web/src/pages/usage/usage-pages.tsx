import { UsageConversations } from "@/components/usage/usage-conversations"
import { personUsage } from "@/components/usage/usage-place"
import { UsageSession } from "@/components/usage/usage-session"
import { UsageSummary } from "@/components/usage/usage-summary"

// A person's own usage: their conversations, one of their sessions, and what their own tasks cost this month.

export function MyUsagePage() {
  return <UsageConversations place={personUsage} />
}

export function MySessionPage() {
  return <UsageSession place={personUsage} />
}

export function MySpendPage() {
  return <UsageSummary place={personUsage} />
}
