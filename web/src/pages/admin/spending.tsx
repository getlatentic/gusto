import { consoleUsage } from "@/components/usage/usage-place"
import { UsageSummary } from "@/components/usage/usage-summary"

// The console's summary: what the whole organization's agents cost.
export function SpendingPage() {
  return <UsageSummary place={consoleUsage} />
}
