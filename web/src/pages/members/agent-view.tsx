import { useSearchParams } from "react-router"

import { AppLink } from "@/components/app-link"
import { Notice } from "@/components/page"
import { agentTabOf, agentTabs, type AgentTab } from "@/domain/members"
import { usePageData } from "@/lib/api"
import { AgentAccess } from "@/pages/members/agent-access"
import { AgentApps } from "@/pages/members/agent-apps"
import { AgentHeader } from "@/pages/members/agent-header"
import { AgentProfile } from "@/pages/members/agent-profile"
import { AgentSkills } from "@/pages/members/agent-skills"
import { BackToMembers, Opening, RoomLinks } from "@/pages/members/member-parts"
import { useMembersOutlet } from "@/pages/members/members-layout"
import type { AgentData, ModelOption } from "@/pages/members/types"

// OpenAgent is the agent the address names, beside the members list.
export function OpenAgent() {
  const loaded = usePageData<AgentData>()
  const offered = useMembersOutlet().list.NewAgent.Offered
  return (
    <Opening loaded={loaded}>
      {(page) => <AgentView page={page} offered={offered} />}
    </Opening>
  )
}

// AgentView is one agent: who it is, and its tabs, each its own address: its profile, who may work with it, its skills, and the
// person's apps it may use where the server offers apps.
export function AgentView({
  page,
  offered,
}: {
  page: AgentData
  offered: ModelOption[]
}) {
  const [params] = useSearchParams()
  const agent = page.Agent
  const tab = agentTabOf(params.get("tab"), page.Apps !== null)
  return (
    <article
      aria-label={agent.Name}
      className="mx-auto grid max-w-[52rem] gap-6 px-8 py-8 max-nav:px-4"
      data-testid="agent-page"
    >
      <BackToMembers />
      {page.Refused && <Notice>{page.Refused}</Notice>}
      <AgentHeader agent={agent} />
      <nav aria-label={`About ${agent.Name}`} data-testid="agent-tabs">
        <ul className="m-0 flex list-none gap-0.5 p-0">
          {agentTabs(page.Apps !== null).map((each) => (
            <li key={each.key}>
              <AppLink
                href={`${agent.Where}?tab=${each.key}`}
                aria-current={each.key === tab ? "page" : undefined}
                className="flex h-8 items-center rounded-full px-3 text-sm font-medium text-muted-foreground no-underline hover:text-foreground aria-[current=page]:bg-sunken aria-[current=page]:text-foreground"
              >
                {each.label}
              </AppLink>
            </li>
          ))}
        </ul>
      </nav>
      <TabBody page={page} tab={tab} offered={offered} />
    </article>
  )
}

function TabBody({
  page,
  tab,
  offered,
}: {
  page: AgentData
  tab: AgentTab
  offered: ModelOption[]
}) {
  const agent = page.Agent
  switch (tab) {
    case "access":
      return <AgentAccess agent={agent} />
    case "skills":
      return <AgentSkills agent={agent} skills={page.Skills} />
    case "apps":
      return page.Apps && <AgentApps agent={agent} apps={page.Apps} />
    default:
      return (
        <>
          <AgentProfile agent={agent} offered={offered} />
          <RoomLinks title="In your rooms" rooms={page.Rooms} />
        </>
      )
  }
}
