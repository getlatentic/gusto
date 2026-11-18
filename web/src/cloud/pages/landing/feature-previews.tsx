import type { ReactNode } from "react"
import {
  ChevronDownIcon,
  ChevronRightIcon,
  CircleCheckIcon,
  KeyRoundIcon,
  LayersIcon,
  LayoutGridIcon,
  LockIcon,
  MonitorIcon,
  UserRoundIcon,
  UsersRoundIcon,
  type LucideIcon,
} from "lucide-react"

import { AgentIcon } from "@/components/agent-icon"

// The small examples of gusto's screens the feature cards show. They are drawings of the product, not live data, so the page
// says nothing they would have to keep true.

const frame =
  "border-line flex h-full flex-col overflow-hidden rounded-xl border bg-canvas text-sm"

// RoomPreview is a room with two people and their agents, one asking an agent and the agent answering.
export function RoomPreview() {
  return (
    <div className={frame}>
      <div className="border-line flex h-12 items-center gap-2.5 border-b px-3">
        <UsersRoundIcon
          aria-hidden="true"
          className="text-accent-ink size-4"
          strokeWidth={1.75}
        />
        <span className="flex-1 truncate font-medium">Product planning</span>
        <span className="flex">
          <Face src="/static/people/priya.webp" />
          <Face src="/static/people/jordan.webp" />
          <span className="border-line border-canvas -ml-2 grid size-7 place-items-center rounded-full border-2 bg-surface">
            <AgentIcon src="/static/agents/codex.svg" className="size-4" />
          </span>
          <span className="text-quiet border-canvas bg-wash -ml-2 grid size-7 place-items-center rounded-full border-2 text-xs">
            +2
          </span>
        </span>
      </div>
      <div className="grid gap-3 p-3">
        <Said who="Sam" face={<Face src="/static/people/sam.webp" first />}>
          <span className="text-accent-ink">@Hermes</span> draft a plan for the
          next release.
        </Said>
        <Said
          who="Hermes"
          face={
            <span className="grid size-7 place-items-center">
              <AgentIcon src="/static/agents/hermes.webp" className="size-6" />
            </span>
          }
        >
          Here’s a draft plan with key milestones…
        </Said>
      </div>
    </div>
  )
}

function Face({ src, first }: { src: string; first?: boolean }) {
  return (
    <img
      className={
        first
          ? "size-7 rounded-full"
          : "border-canvas -ml-2 size-7 rounded-full border-2 first:ml-0"
      }
      src={src}
      alt=""
    />
  )
}

function Said({
  who,
  face,
  children,
}: {
  who: string
  face: ReactNode
  children: ReactNode
}) {
  return (
    <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-2.5 text-left">
      <span className="row-span-2">{face}</span>
      <span className="font-medium">{who}</span>
      <span className="text-quiet">{children}</span>
    </div>
  )
}

// ComputerPreview is a person's own cloud computer with an agent running in it.
export function ComputerPreview() {
  return (
    <div className={frame}>
      <div className="border-line flex h-12 items-center gap-2.5 border-b px-3">
        <MonitorIcon aria-hidden="true" className="size-4" strokeWidth={1.75} />
        <span className="flex-1 truncate font-medium">Your cloud computer</span>
        <span className="bg-solid/10 text-accent-ink flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium">
          <LockIcon aria-hidden="true" className="size-3" /> Private
        </span>
      </div>
      <div className="grid flex-1 content-start gap-3 p-4 text-left">
        <code className="text-quiet font-mono text-xs">
          &gt; Running agent…
        </code>
        <span className="bg-line h-2.5 w-3/4 rounded-full" />
        <span className="bg-line h-2.5 w-1/2 rounded-full" />
      </div>
    </div>
  )
}

// ControlsPreview is the settings an admin keeps: a budget per person, the agents and models allowed, their own AI keys, and the
// tools agents may use.
export function ControlsPreview() {
  return (
    <ul className={`${frame} m-0 list-none divide-y p-0`}>
      <Control icon={UserRoundIcon} label="Budget per person">
        <span className="border-line-strong flex items-center gap-1 rounded-lg border px-2 py-0.5 font-medium">
          $50 / mo <ChevronDownIcon aria-hidden="true" className="size-3.5" />
        </span>
      </Control>
      <Control icon={LayoutGridIcon} label="Allowed agents & models">
        <Value>6 selected</Value>
      </Control>
      <Control icon={KeyRoundIcon} label="Your own AI keys">
        <span className="bg-solid/10 text-accent-ink flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium">
          Connected <CircleCheckIcon aria-hidden="true" className="size-3.5" />
        </span>
      </Control>
      <Control icon={LayersIcon} label="Tools, skills & MCP servers">
        <Value>8 allowed</Value>
      </Control>
    </ul>
  )
}

function Control({
  icon: Icon,
  label,
  children,
}: {
  icon: LucideIcon
  label: string
  children: ReactNode
}) {
  return (
    <li className="border-line flex min-h-11 flex-1 items-center gap-2.5 px-3 py-2">
      <Icon
        aria-hidden="true"
        className="text-quiet size-4 flex-none"
        strokeWidth={1.75}
      />
      <span className="min-w-0 flex-1 text-left">{label}</span>
      {children}
    </li>
  )
}

function Value({ children }: { children: string }) {
  return (
    <span className="text-quiet flex items-center gap-0.5 whitespace-nowrap">
      {children}
      <ChevronRightIcon aria-hidden="true" className="size-3.5" />
    </span>
  )
}
