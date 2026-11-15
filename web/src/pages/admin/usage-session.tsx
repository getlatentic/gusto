import { consoleUsage } from "@/components/usage/usage-place"
import { UsageSession } from "@/components/usage/usage-session"

// A session of anyone's, in the console.
export function UsageSessionPage() {
  return <UsageSession place={consoleUsage} />
}
