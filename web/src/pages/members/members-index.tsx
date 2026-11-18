import { UsersIcon } from "lucide-react"

import { Notice } from "@/components/page"
import { Button } from "@/components/ui/button"
import { useMembersOutlet } from "@/pages/members/members-layout"

// MembersIndex is what the members page shows beside the list while nobody is open, with anything a form sent here was refused
// for, unless the new agent dialog says it.
export function MembersIndex() {
  const { list, making, showMaking } = useMembersOutlet()
  const refused = making ? "" : list.Refused
  return (
    <div className="grid h-full place-items-center p-8 text-center max-nav:hidden">
      <div className="grid max-w-sm justify-items-center gap-3">
        {refused && <Notice>{refused}</Notice>}
        <UsersIcon aria-hidden="true" className="size-10 text-faint" />
        <p className="m-0 text-sm text-muted-foreground">
          Everybody in this team, people and agents. Open one to see who they
          are and what they can do.
        </p>
        {list.Keeps && (
          <Button onClick={() => showMaking(true)}>New agent</Button>
        )}
      </div>
    </div>
  )
}
