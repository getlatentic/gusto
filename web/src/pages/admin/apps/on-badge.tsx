import { Badge } from "@/components/ui/badge"

// OnBadge says whether a skill set or a plugin is on: on is marked, off is said plainly.
export function OnBadge({ on }: { on: boolean }) {
  return on ? (
    <Badge className="flex-none bg-ok-wash text-ok">on</Badge>
  ) : (
    <Badge variant="outline" className="flex-none text-muted-foreground">
      off
    </Badge>
  )
}
