import { createContext, useContext, type ReactNode } from "react"
import {
  FolderIcon,
  HashIcon,
  LayoutGridIcon,
  MessagesSquareIcon,
  SettingsIcon,
  UsersRoundIcon,
  type LucideIcon,
} from "lucide-react"

import { AgentIcon } from "@/components/agent-icon"
import { cn } from "@/lib/utils"

// The parts the demo's scenes are drawn with, styled as gusto's own pages are: the icon rail, the side panel beside it, faces
// and messages. On a wide page every scene is drawn at one size, demoWidth by demoHeight, and scaled to fit. On a page narrower
// than compactWidth it is drawn compactHeight tall, as gusto's phone pages are: the page alone, without the rail and side panel,
// its text at its usual size. It is drawn at the page's own width, but never narrower than phoneWidth, and is scaled to fit a
// narrower page, so a small phone sees the same page smaller rather than squeezed.

export const demoWidth = 1112
export const demoHeight = 580
export const compactWidth = 640
export const compactHeight = 560
export const phoneWidth = 400

const Compact = createContext(false)
export const CompactDemo = Compact.Provider

// useCompact is whether the scene is being drawn as a phone page.
export function useCompact(): boolean {
  return useContext(Compact)
}

export const people = {
  sam: { name: "Sam", face: "/static/people/sam.webp" },
  priya: { name: "Priya", face: "/static/people/priya.webp" },
  jordan: { name: "Jordan", face: "/static/people/jordan.webp" },
  maya: { name: "Maya", face: "/static/people/maya.webp" },
}

export const agents = {
  hermes: { name: "Hermes", mark: "/static/agents/hermes.webp" },
  claude: { name: "Claude Code", mark: "/static/agents/claude.svg" },
}

// Shell is one of gusto's pages: the rail of places, a side panel, and the page itself.
export function Shell({
  place,
  side,
  children,
}: {
  place: "chat" | "admin"
  side: ReactNode
  children: ReactNode
}) {
  const places: [LucideIcon, string][] = [
    [MessagesSquareIcon, "chat"],
    [FolderIcon, "files"],
    [UsersRoundIcon, "members"],
    [LayoutGridIcon, "admin"],
  ]
  if (useCompact()) {
    return (
      <main className="relative flex h-full min-w-0 flex-col bg-background text-foreground">
        {children}
      </main>
    )
  }
  return (
    <div className="grid h-full grid-cols-[52px_232px_minmax(0,1fr)] bg-background text-foreground">
      <nav className="flex flex-col items-center gap-1 bg-rail pt-5 pb-4">
        <img className="mb-2 size-6" src="/static/mark.svg" alt="" />
        {places.map(([Icon, name]) => (
          <span
            key={name}
            className={cn(
              "grid size-10 place-items-center rounded-lg text-muted-foreground",
              name === place && "bg-surface text-foreground shadow-sm"
            )}
          >
            <Icon className="size-4" strokeWidth={1.75} />
          </span>
        ))}
        <span className="flex-1" />
        <img className="size-7 rounded-full" src={people.sam.face} alt="" />
      </nav>
      <aside className="flex flex-col gap-1 bg-side px-3 py-4">{side}</aside>
      <main className="relative flex min-w-0 flex-col">{children}</main>
    </div>
  )
}

export function SideTitle({ children }: { children: ReactNode }) {
  return (
    <p className="m-0 mb-2 flex h-8 items-center px-3 font-heading text-base font-semibold">
      {children}
    </p>
  )
}

export function SideRow({
  icon: Icon = HashIcon,
  current,
  target,
  children,
}: {
  icon?: LucideIcon
  current?: boolean
  target?: string
  children: ReactNode
}) {
  return (
    <span
      data-demo-target={target}
      className={cn(
        "flex h-9 items-center gap-2 rounded-md px-3 text-sm text-muted-foreground",
        current && "bg-surface font-medium text-foreground shadow-sm"
      )}
    >
      <Icon className="size-4 flex-none text-subtle" strokeWidth={1.75} />
      <span className="truncate">{children}</span>
    </span>
  )
}

// PageHeader is the bar along the top of a page: its name, and what can be done from there.
export function PageHeader({
  title,
  children,
}: {
  title: ReactNode
  children?: ReactNode
}) {
  return (
    <header className="flex h-14 flex-none items-center gap-2 px-6 group-data-[compact=true]:px-4">
      <span className="flex min-w-0 flex-1 items-center gap-2 text-sm font-semibold">
        {title}
      </span>
      {children}
    </header>
  )
}

export function HeaderButton({
  icon: Icon = SettingsIcon,
  active,
  target,
  children,
}: {
  icon?: LucideIcon
  active?: boolean
  target?: string
  children: ReactNode
}) {
  return (
    <span
      data-demo-target={target}
      className={cn(
        "flex h-8 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium text-muted-foreground",
        active && "bg-sunken text-foreground"
      )}
    >
      <Icon className="size-4" strokeWidth={1.75} />
      {children}
    </span>
  )
}

// Message is one thing said in a room, by a person or an agent, as a room shows it: a face, a name and what was said.
export function Message({
  who,
  face,
  agent,
  note,
  children,
}: {
  who: string
  face: string
  agent?: boolean
  note?: string
  children: ReactNode
}) {
  return (
    <div className="grid [animation:demo-in_240ms_ease-out] grid-cols-[32px_minmax(0,1fr)] gap-x-3">
      <span className="row-span-2 grid size-8 place-items-center overflow-hidden rounded-full bg-sunken">
        {agent ? (
          <AgentIcon src={face} className="size-5" />
        ) : (
          <img className="size-8" src={face} alt="" />
        )}
      </span>
      <span className="flex items-center gap-2 text-xs font-medium">
        {who}
        {agent && (
          <span className="rounded-full bg-sunken px-1.5 py-px text-[10px] font-normal text-muted-foreground">
            agent
          </span>
        )}
        {note && <span className="font-normal text-subtle">{note}</span>}
      </span>
      <div className="grid gap-2 text-sm leading-6">{children}</div>
    </div>
  )
}

export function Mention({ children }: { children: string }) {
  return <span className="text-accent-ink font-medium">{children}</span>
}
