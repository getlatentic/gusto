import {
  FileTextIcon,
  FolderIcon,
  LockIcon,
  MonitorIcon,
  UsersRoundIcon,
} from "lucide-react"

import {
  agents,
  HeaderButton,
  Mention,
  Message,
  PageHeader,
  people,
  Shell,
  SideRow,
  SideTitle,
} from "@/cloud/pages/landing/demo/demo-parts"
import type { Frame, Scene, Step } from "@/cloud/pages/landing/demo/script"
import { cn } from "@/lib/utils"

interface Computer {
  open: boolean
  lines: number
  done: boolean
}

const log = [
  "$ cd ~/product-planning",
  "Reading roadmap.md",
  "Reading notes/q3-goals.md",
  "Writing release-plan.md",
  "Checking the plan against the roadmap",
]

// computerScene: Sam opens the private cloud computer Hermes is working in and watches it read, write and finish.
export const computerScene: Scene<Computer> = {
  length: 10500,
  start: { open: false, lines: 0, done: false },
  steps: [
    { at: 0, cursor: "talk" },
    { at: 600, cursor: "computer", label: "Open Hermes’s computer" },
    { at: 1500, click: true, set: { open: true } },
    { at: 2000, cursor: "log" },
    ...log.map((_, at): Step<Computer> => ({
      at: 2300 + at * 750,
      set: { lines: at + 1 },
    })),
    { at: 6400, set: { done: true } },
    { at: 7000, cursor: "plan", label: "release-plan.md" },
  ],
}

export function ComputerScene({ frame }: { frame: Frame<Computer> }) {
  const computer = frame.state
  return (
    <Shell
      place="chat"
      side={
        <>
          <SideTitle>Acme</SideTitle>
          <SideRow current>Product planning</SideRow>
          <SideRow>Launch plan</SideRow>
          <SideRow>Support inbox</SideRow>
          <SideRow icon={UsersRoundIcon}>Priya</SideRow>
        </>
      }
    >
      <PageHeader title="Product planning">
        <HeaderButton icon={UsersRoundIcon}>5</HeaderButton>
        <HeaderButton
          icon={MonitorIcon}
          active={computer.open}
          target="computer"
        >
          Computer
        </HeaderButton>
      </PageHeader>
      <div className="flex min-h-0 flex-1">
        <div
          className={cn(
            "flex min-w-0 flex-1 flex-col justify-end gap-5 px-8 pb-6 group-data-[compact=true]:px-4",
            computer.open && "group-data-[compact=true]:hidden"
          )}
          data-demo-target="talk"
        >
          <Message who="Sam" face={people.sam.face} note="9:42">
            <span>
              <Mention>@Hermes</Mention> draft a plan for the next release
            </span>
          </Message>
          <p className="m-0 flex items-center gap-2 pl-11 text-xs text-muted-foreground">
            {computer.done ? (
              "Hermes finished in 41s"
            ) : (
              <>
                <span className="size-3 animate-spin rounded-full border-2 border-input border-t-foreground" />
                Hermes is working on its computer
              </>
            )}
          </p>
        </div>
        {computer.open && <ComputerPanel computer={computer} />}
      </div>
    </Shell>
  )
}

function ComputerPanel({ computer }: { computer: Computer }) {
  return (
    <section className="m-3 mt-0 flex w-[400px] flex-none [animation:demo-in_240ms_ease-out] flex-col self-start overflow-hidden rounded-xl border bg-surface group-data-[compact=true]:w-auto group-data-[compact=true]:flex-1">
      <header className="flex h-11 items-center gap-2 border-b px-4 text-sm font-medium">
        <MonitorIcon className="size-4" strokeWidth={1.75} />
        {agents.hermes.name}’s computer
        <span className="flex-1" />
        <span className="bg-solid/10 text-accent-ink flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium">
          <LockIcon className="size-3" /> Private
        </span>
      </header>
      <div
        data-demo-target="log"
        className="carbon bg-canvas text-quiet grid h-52 content-start gap-1.5 p-4 font-mono text-xs"
      >
        {log.slice(0, computer.lines).map((line, at) => (
          <span
            key={line}
            className={cn(
              "[animation:demo-in_200ms_ease-out]",
              at === 0 && "text-ink"
            )}
          >
            {line}
          </span>
        ))}
        {computer.done && (
          <span className="text-ink [animation:demo-in_200ms_ease-out] font-medium">
            ✓ Done in 41s · $0.02
          </span>
        )}
      </div>
      <ul className="m-0 grid list-none gap-0.5 border-t p-2 text-sm">
        <File icon={FolderIcon} name="notes" />
        <File name="roadmap.md" />
        {computer.done && <File name="release-plan.md" fresh target="plan" />}
      </ul>
    </section>
  )
}

function File({
  icon: Icon = FileTextIcon,
  name,
  fresh,
  target,
}: {
  icon?: typeof FileTextIcon
  name: string
  fresh?: boolean
  target?: string
}) {
  return (
    <li
      data-demo-target={target}
      className={cn(
        "flex h-8 items-center gap-2 rounded-md px-2",
        fresh && "bg-solid/10 [animation:demo-in_240ms_ease-out] font-medium"
      )}
    >
      <Icon className="size-4 text-subtle" strokeWidth={1.75} />
      {name}
      {fresh && <span className="text-accent-ink ml-auto text-xs">new</span>}
    </li>
  )
}
