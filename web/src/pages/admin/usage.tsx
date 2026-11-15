import { UsageConversations } from "@/components/usage/usage-conversations"
import { consoleUsage } from "@/components/usage/usage-place"

// The console's conversations: everyone's.
export function UsagePage() {
  return <UsageConversations place={consoleUsage} />
}
