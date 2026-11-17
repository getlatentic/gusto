import { useState, type ReactNode } from "react"
import { BotIcon, SquarePenIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  memberTabOf,
  memberTabs,
  shownIn,
  type MemberTab,
} from "@/domain/members"
import { cn } from "@/lib/utils"
import { FlipList } from "@/motion/flip-list"
import { AgentFace, PersonFace } from "@/pages/members/faces"
import type { MembersData } from "@/pages/members/types"

// The tab chosen last is kept on this device for the next page.
const keptTabKey = "gusto-member-tab"

function keptTab(): MemberTab {
  try {
    return memberTabOf(localStorage.getItem(keptTabKey))
  } catch {
    return "all"
  }
}

function keepTab(tab: MemberTab) {
  try {
    localStorage.setItem(keptTabKey, tab)
  } catch {
    // A browser that keeps nothing opens on All next time.
  }
}

// MembersList is everybody in the workspace, people and agents, with tabs that show everybody, only the people or only the agents.
// On a narrow window it folds away while somebody is open.
export function MembersList({
  page,
  here,
  folded,
}: {
  page: MembersData
  here: string
  folded: boolean
}) {
  const [tab, setTab] = useState(keptTab)
  const shown = shownIn(tab)
  const choose = (next: unknown) => {
    const chosen = memberTabOf(String(next))
    setTab(chosen)
    keepTab(chosen)
  }
  return (
    <nav
      aria-label="Members"
      data-testid="members-list"
      className={cn(
        "flex w-80 flex-none flex-col gap-3 bg-side p-3",
        folded ? "max-nav:hidden" : "max-nav:w-full"
      )}
    >
      <Tabs value={tab} onValueChange={choose} className="min-h-0 flex-1 gap-3">
        <TabsList aria-label="Which members" data-testid="member-tabs">
          {memberTabs.map((each) => (
            <TabsTrigger key={each.key} value={each.key}>
              {each.label}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value={tab} className="-mx-1 min-h-0 overflow-y-auto px-1">
          <FlipList>
            <ul className="m-0 grid list-none gap-px p-0">
              {shown.people &&
                page.People.map((person) => (
                  <li key={person.Who} data-flip-id={person.Who}>
                    <MemberLink href={person.Where} here={here}>
                      <PersonFace initials={person.Initials} />
                      <Lines name={person.Name} under={person.Role} />
                    </MemberLink>
                  </li>
                ))}
              {shown.agents &&
                page.Agents.map((agent) => (
                  <li key={agent.ID} data-flip-id={agent.ID}>
                    <MemberLink href={agent.Where} here={here}>
                      <AgentFace mark={agent.Mark} name={agent.Name} />
                      <Lines
                        name={agent.Name}
                        under={agent.Specialty || agent.Engine}
                        agent
                      />
                    </MemberLink>
                  </li>
                ))}
            </ul>
          </FlipList>
        </TabsContent>
      </Tabs>
      <AppLink
        href="/presets"
        className="flex h-9 items-center gap-2 rounded-lg px-2 text-sm text-muted-foreground no-underline hover:bg-hover hover:text-foreground"
      >
        <SquarePenIcon aria-hidden="true" className="size-4" /> Ready-made roles
        and presets
      </AppLink>
    </nav>
  )
}

function MemberLink({
  href,
  here,
  children,
}: {
  href: string
  here: string
  children: ReactNode
}) {
  return (
    <AppLink
      href={href}
      data-testid="member-row"
      data-href={href}
      aria-current={href === here ? "page" : undefined}
      className="flex items-center gap-3 rounded-lg px-2 py-2 text-foreground no-underline hover:bg-hover aria-[current=page]:bg-surface aria-[current=page]:shadow-sm"
    >
      {children}
    </AppLink>
  )
}

function Lines({
  name,
  under,
  agent = false,
}: {
  name: string
  under: string
  agent?: boolean
}) {
  return (
    <span className="grid min-w-0">
      <span className="flex min-w-0 items-center gap-1.5 text-sm font-medium">
        <span className="truncate">{name}</span>
        {agent && (
          <BotIcon
            className="size-3.5 flex-none text-subtle"
            aria-label="Agent"
            role="img"
          />
        )}
      </span>
      <span className="truncate text-xs text-subtle">{under}</span>
    </span>
  )
}
