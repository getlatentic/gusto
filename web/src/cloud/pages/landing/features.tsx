import type { ReactNode } from "react"
import {
  CloudIcon,
  MessagesSquareIcon,
  MonitorIcon,
  ServerIcon,
  ShieldCheckIcon,
  type LucideIcon,
} from "lucide-react"

import { AgentMarks } from "@/cloud/pages/landing/agent-marks"
import {
  ComputerPreview,
  ControlsPreview,
  RoomPreview,
} from "@/cloud/pages/landing/feature-previews"
import { Band, BandText, BandTitle } from "@/cloud/pages/landing/landing-parts"

// Features is what a team gets: the agents gusto runs, three things it does for them, each with a small example of the screen,
// and where it can be run.
export function Features() {
  return (
    <Band id="features" wash>
      <div className="grid justify-items-center gap-4 text-center">
        <span className="bg-solid/10 text-accent-ink rounded-full px-4 py-1.5 text-sm font-medium">
          Supported agents
        </span>
        <BandTitle>Your agents, your choice.</BandTitle>
        <BandText>
          Bring the agents your team already uses into one workspace, then
          decide the models, tools and access they’re allowed to use.
        </BandText>
      </div>
      <AgentMarks />
      <ul className="m-0 mt-12 grid list-none grid-cols-[minmax(0,1fr)] gap-5 p-0 min-[60rem]:grid-cols-3 min-[60rem]:gap-y-3">
        <Feature
          icon={MessagesSquareIcon}
          title="Collaborate in shared rooms"
          text="Work with teammates and AI agents in one room, sharing context, files and outcomes."
        >
          <RoomPreview />
        </Feature>
        <Feature
          icon={MonitorIcon}
          title="Private cloud computers"
          text="Each person gets an isolated cloud computer for their agents, apart from the workspace."
        >
          <ComputerPreview />
        </Feature>
        <Feature
          icon={ShieldCheckIcon}
          title="Stay in control"
          text="Set budgets, approve agents and models, and choose the tools, skills and MCP servers."
        >
          <ControlsPreview />
        </Feature>
      </ul>
      <DeployWhereItFits />
    </Band>
  )
}

// Feature is one card. Side by side, the cards share their rows (icon, title, text, preview) through a subgrid, so a title that
// wraps moves the others' text and previews down with it and every edge lines up.
function Feature({
  icon: Icon,
  title,
  text,
  children,
}: {
  icon: LucideIcon
  title: string
  text: string
  children: ReactNode
}) {
  return (
    <li
      data-reveal
      className="border-line-strong flex flex-col gap-3 rounded-2xl border bg-surface p-6 min-[60rem]:row-span-4 min-[60rem]:grid min-[60rem]:grid-rows-subgrid"
    >
      <Icon
        aria-hidden="true"
        className="text-accent-ink size-7"
        strokeWidth={1.75}
      />
      <h3 className="m-0 text-xl font-semibold">{title}</h3>
      <BandText>{text}</BandText>
      <div className="mt-auto pt-3 min-[60rem]:mt-0" aria-hidden="true">
        {children}
      </div>
    </li>
  )
}

const places: { name: string; icon: LucideIcon | "mark"; ours?: boolean }[] = [
  { name: "Hosted by gusto", icon: "mark", ours: true },
  { name: "Your own cloud", icon: CloudIcon },
  { name: "Self-hosted", icon: ServerIcon },
]

function DeployWhereItFits() {
  return (
    <div className="border-line mt-12 flex flex-wrap items-center justify-center gap-3 border-t pt-10">
      <span className="mr-2 text-base font-medium">Deploy where it fits:</span>
      <ul
        aria-label="Deploy where it fits"
        className="m-0 flex list-none flex-wrap justify-center gap-3 p-0"
      >
        {places.map(({ name, icon: Icon, ours }) => (
          <li
            key={name}
            className={
              ours
                ? "bg-solid/10 text-accent-ink flex h-11 items-center gap-2.5 rounded-xl px-4 text-sm font-medium"
                : "border-line-strong flex h-11 items-center gap-2.5 rounded-xl border bg-surface px-4 text-sm font-medium"
            }
          >
            {Icon === "mark" ? (
              <img className="size-5" src="/static/mark.svg" alt="" />
            ) : (
              <Icon aria-hidden="true" className="size-5" strokeWidth={1.75} />
            )}
            {name}
          </li>
        ))}
      </ul>
    </div>
  )
}
