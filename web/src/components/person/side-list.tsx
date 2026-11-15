import { useState } from "react"
import { PlusIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { ProjectGroups } from "@/components/person/project-groups"
import { SideRow, sideItem } from "@/components/person/side-row"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  holds,
  isSideTab,
  roomsForYou,
  unreadRooms,
  type RailChat,
  type RailList,
  type SideTab,
} from "@/domain/side-list"

const tabKey = "gusto-side-tab"

// keptTab is the tab chosen last on this device, since moving between conversations should not lose a person's place.
function keptTab(): SideTab {
  try {
    const kept = localStorage.getItem(tabKey)
    return isSideTab(kept) ? kept : "home"
  } catch {
    return "home"
  }
}

function keepTab(tab: SideTab) {
  try {
    localStorage.setItem(tabKey, tab)
  } catch {
    // A browser that keeps nothing starts at Home next time.
  }
}

const panel = "m-0 grid min-h-0 flex-1 content-start gap-px overflow-y-auto"

// SideList is a person's conversations: their rooms and projects, every conversation newest first, the rooms with something
// unread, and those that mention them.
export function SideList({
  rail,
  here,
  onGo,
}: {
  rail: RailList
  here: string
  onGo?: () => void
}) {
  const [tab, setTab] = useState<SideTab>(keptTab)
  const unread = unreadRooms(rail)
  const forYou = roomsForYou(rail)
  return (
    <Tabs
      value={tab}
      onValueChange={(next: SideTab) => {
        setTab(next)
        keepTab(next)
      }}
      className="flex min-h-0 flex-1 flex-col gap-2"
      data-testid="side-list"
    >
      <TabsList
        aria-label="Which conversations"
        className="w-full justify-start"
      >
        <TabsTrigger value="home">Home</TabsTrigger>
        <TabsTrigger value="recent">Recent</TabsTrigger>
        <TabsTrigger value="unread">
          Unread
          {rail.Unread > 0 && <Count n={rail.Unread} />}
        </TabsTrigger>
        <TabsTrigger value="me">
          @me
          {rail.ForYou > 0 && <Count n={rail.ForYou} />}
        </TabsTrigger>
      </TabsList>
      <TabsContent value="home" className="min-h-0 flex-1 overflow-y-auto">
        <Home rail={rail} here={here} onGo={onGo} />
      </TabsContent>
      <TabsContent value="recent" className={panel}>
        <nav aria-label="Recent conversations">
          <Rows
            chats={rail.Latest ?? []}
            here={here}
            onGo={onGo}
            empty="No conversations yet."
          />
        </nav>
      </TabsContent>
      <TabsContent value="unread" className={panel}>
        <nav aria-label="Unread rooms">
          <Rows
            chats={unread}
            here={here}
            onGo={onGo}
            empty="You have read everything."
          />
        </nav>
      </TabsContent>
      <TabsContent value="me" className={panel}>
        <nav aria-label="Rooms that mention you">
          <Rows
            chats={forYou}
            here={here}
            onGo={onGo}
            empty="Nobody has mentioned you since you last looked."
          />
        </nav>
      </TabsContent>
    </Tabs>
  )
}

function Count({ n }: { n: number }) {
  return (
    <>
      {" "}
      <span className="text-xs text-subtle tabular-nums">{n}</span>
    </>
  )
}

function Rows({
  chats,
  here,
  onGo,
  empty,
}: {
  chats: RailChat[]
  here: string
  onGo?: () => void
  empty: string
}) {
  if (chats.length === 0) {
    return <p className="m-0 px-3 py-2 text-sm text-subtle">{empty}</p>
  }
  return (
    <ul className="m-0 grid list-none gap-px p-0">
      {chats.map((chat) => (
        <li key={chat.Where}>
          <SideRow chat={chat} here={here} onGo={onGo} />
        </li>
      ))}
    </ul>
  )
}

function Home({
  rail,
  here,
  onGo,
}: {
  rail: RailList
  here: string
  onGo?: () => void
}) {
  const archived = rail.Archived ?? []
  return (
    <nav aria-label="Conversations" className="grid content-start gap-2">
      {(rail.Direct ?? []).length > 0 && (
        <section aria-labelledby="side-direct" className="grid gap-px">
          <h2
            id="side-direct"
            className="m-0 flex h-9 items-center px-3 text-sm font-semibold"
          >
            Direct messages
          </h2>
          <Rows chats={rail.Direct ?? []} here={here} onGo={onGo} empty="" />
        </section>
      )}
      <section
        aria-labelledby="side-rooms"
        className="grid gap-px"
        data-testid="side-rooms"
      >
        <header className="flex h-9 items-center justify-between pr-1 pl-3">
          <h2 id="side-rooms" className="m-0 text-sm font-semibold">
            Rooms
          </h2>
          <AppLink
            href="/rooms/new"
            title="New room"
            aria-label="New room"
            className="grid size-6 place-items-center rounded-sm text-muted-foreground hover:bg-surface hover:text-foreground hover:shadow-sm [&>svg]:size-4"
            onClick={onGo}
          >
            <PlusIcon />
          </AppLink>
        </header>
        {(rail.Rooms ?? []).length > 0 && (
          <Rows chats={rail.Rooms ?? []} here={here} onGo={onGo} empty="" />
        )}
      </section>
      {archived.length > 0 && (
        <Collapsible
          defaultOpen={holds(archived, here)}
          data-testid="side-archived"
        >
          <CollapsibleTrigger
            className={`${sideItem} cursor-pointer justify-between border-0 bg-transparent`}
          >
            Archived rooms{" "}
            <span className="tabular-nums">{archived.length}</span>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <Rows chats={archived} here={here} onGo={onGo} empty="" />
          </CollapsibleContent>
        </Collapsible>
      )}
      <section aria-labelledby="side-projects" className="grid gap-px">
        <h2
          id="side-projects"
          className="m-0 flex h-9 items-center px-3 text-sm font-semibold"
        >
          Projects
        </h2>
        <ProjectGroups groups={rail.Recent ?? []} here={here} onGo={onGo} />
        {(rail.Older ?? []).length > 0 && (
          <Collapsible>
            <CollapsibleTrigger
              className={`${sideItem} cursor-pointer justify-between border-0 bg-transparent`}
            >
              Older projects{" "}
              <span className="tabular-nums">{rail.Older!.length}</span>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <ProjectGroups
                groups={rail.Older ?? []}
                here={here}
                onGo={onGo}
              />
            </CollapsibleContent>
          </Collapsible>
        )}
      </section>
    </nav>
  )
}
