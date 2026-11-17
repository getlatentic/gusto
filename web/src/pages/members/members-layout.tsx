import { useRef, useState } from "react"
import { Outlet, useLocation, useMatch, useOutletContext } from "react-router"

import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody } from "@/components/page"
import { listSearch, openIn } from "@/domain/members"
import { dataOf, pagePath, useData } from "@/lib/api"
import { usePageEnter } from "@/motion/use-page-enter"
import { AddMenu } from "@/pages/members/add-menu"
import { InviteDialog } from "@/pages/members/invite-dialog"
import { MembersList } from "@/pages/members/members-list"
import { NewAgentDialog } from "@/pages/members/new-agent-dialog"
import type { MembersData } from "@/pages/members/types"
import { useNewAgentDialog } from "@/pages/members/use-new-agent-dialog"
import { useChildPath } from "@/router/use-child-path"

// MembersOutlet is what the members layout hands the page open beside the list: the list's data, and the new agent dialog.
export interface MembersOutlet {
  list: MembersData
  making: boolean
  showMaking: (open: boolean) => void
}

export function useMembersOutlet(): MembersOutlet {
  return useOutletContext<MembersOutlet>()
}

// MembersLayout is everybody in the workspace, people and agents, in a list beside whichever of them is open. The list and its
// dialogs are read once and stay drawn while the page beside them changes. An agent is made here once, and from here it is added
// to rooms or talked to on its own.
export function MembersLayout() {
  const where = useLocation()
  const atList = useMatch("/members") !== null
  const loaded = useData<MembersData>(
    pagePath({ pathname: "/members", search: listSearch(where) })
  )
  const [making, showMaking] = useNewAgentDialog()
  const [inviting, setInviting] = useState(false)
  const detail = useRef<HTMLDivElement>(null)
  usePageEnter(detail, useChildPath("members"))
  const list = dataOf(loaded)
  if (!list) {
    return (
      <>
        <PageBar title="Members" />
        <PageBody>
          <Loading loaded={loaded}>{() => null}</Loading>
        </PageBody>
      </>
    )
  }
  const open = openIn([...list.People, ...list.Agents], where.pathname)
  return (
    <>
      <PageBar
        title={open?.Name ?? "Members"}
        crumbs={atList ? [] : [{ label: "Members", href: "/members" }]}
      >
        <AddMenu
          invites={list.Invite !== null}
          keeps={list.Keeps}
          onInvite={() => setInviting(true)}
          onNewAgent={() => showMaking(true)}
        />
      </PageBar>
      <div className="flex min-h-0 flex-1">
        <MembersList page={list} here={where.pathname} folded={!atList} />
        <div
          ref={detail}
          className="min-w-0 flex-1 overflow-y-auto"
          data-testid="member-detail"
        >
          {list.Sent && (
            <div className="mx-8 mt-6 max-nav:mx-4">
              <Notice tone="plain">
                The invitation was sent. The person is mailed a link that works
                for seven days.
              </Notice>
            </div>
          )}
          <Outlet
            context={{ list, making, showMaking } satisfies MembersOutlet}
          />
        </div>
      </div>
      {list.Keeps && (
        <NewAgentDialog
          page={list}
          open={making}
          onClose={() => showMaking(false)}
        />
      )}
      {list.Invite && (
        <InviteDialog
          invite={list.Invite}
          open={inviting}
          onClose={() => setInviting(false)}
        />
      )}
    </>
  )
}
