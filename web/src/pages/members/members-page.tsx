import { useState } from "react"
import { UsersIcon } from "lucide-react"
import { useLocation } from "react-router"

import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody } from "@/components/page"
import { Button } from "@/components/ui/button"
import { dataOf, usePageData } from "@/lib/api"
import { AddMenu } from "@/pages/members/add-menu"
import { AgentView } from "@/pages/members/agent-view"
import { InviteDialog } from "@/pages/members/invite-dialog"
import { MembersList } from "@/pages/members/members-list"
import { NewAgentDialog } from "@/pages/members/new-agent-dialog"
import { PersonView } from "@/pages/members/person-view"
import type { MembersPageData } from "@/pages/members/types"
import { useNewAgentDialog } from "@/pages/members/use-new-agent-dialog"

// MembersPage is everybody in the workspace, people and agents, in a list beside whichever of them is open. An agent is made
// here once, and from here it is added to rooms or talked to on its own. The list and its dialogs are on every members page, so
// the page stays drawn while the next one is read.
export function MembersPage() {
  const loaded = usePageData<MembersPageData>()
  const here = useLocation().pathname
  const [making, showMaking] = useNewAgentDialog()
  const [inviting, setInviting] = useState(false)
  const page = dataOf(loaded)
  if (!page) {
    return (
      <>
        <PageBar title="Members" />
        <PageBody>
          <Loading loaded={loaded}>{() => null}</Loading>
        </PageBody>
      </>
    )
  }
  const open =
    "Agent" in page ? page.Agent : "Person" in page ? page.Person : undefined
  return (
    <>
      <PageBar
        title={open?.Name ?? "Members"}
        crumbs={open ? [{ label: "Members", href: "/members" }] : []}
      >
        <AddMenu
          invites={page.Invite !== null}
          keeps={page.Keeps}
          onInvite={() => setInviting(true)}
          onNewAgent={() => showMaking(true)}
        />
      </PageBar>
      <div className="flex min-h-0 flex-1">
        <MembersList page={page} here={here} folded={open !== undefined} />
        <div
          className="min-w-0 flex-1 overflow-y-auto"
          data-testid="member-detail"
        >
          {page.Sent && (
            <div className="mx-8 mt-6 max-nav:mx-4">
              <Notice tone="plain">
                The invitation was sent. The person is mailed a link that works
                for seven days.
              </Notice>
            </div>
          )}
          {"Agent" in page ? (
            <AgentView page={page} />
          ) : "Person" in page ? (
            <PersonView page={page} />
          ) : (
            <Nobody
              page={page}
              refused={making ? "" : page.Refused}
              onNewAgent={() => showMaking(true)}
            />
          )}
        </div>
      </div>
      {page.Keeps && (
        <NewAgentDialog
          page={page}
          open={making}
          onClose={() => showMaking(false)}
        />
      )}
      {page.Invite && (
        <InviteDialog
          invite={page.Invite}
          open={inviting}
          onClose={() => setInviting(false)}
        />
      )}
    </>
  )
}

// Nobody is what the members page shows while nobody is open, with anything a form sent here was refused for.
function Nobody({
  page,
  refused,
  onNewAgent,
}: {
  page: MembersPageData
  refused: string
  onNewAgent: () => void
}) {
  return (
    <div className="grid h-full place-items-center p-8 text-center max-nav:hidden">
      <div className="grid max-w-sm justify-items-center gap-3">
        {refused && <Notice>{refused}</Notice>}
        <UsersIcon aria-hidden="true" className="size-10 text-faint" />
        <p className="m-0 text-sm text-muted-foreground">
          Everybody in this workspace, people and agents. Open one to see who
          they are and what they can do.
        </p>
        {page.Keeps && <Button onClick={onNewAgent}>New agent</Button>}
      </div>
    </div>
  )
}
