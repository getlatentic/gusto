import { ArrowUpIcon, FileTextIcon, UsersRoundIcon } from "lucide-react"

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
import type { Frame, Scene } from "@/cloud/pages/landing/demo/script"
import { cn } from "@/lib/utils"

interface Room {
  draft: string
  sent: boolean
  working: boolean
  replied: boolean
  followUp: boolean
}

const ask = "@Hermes draft a plan for the next release"

// roomScene: Sam asks Hermes for a plan in a shared room, Hermes writes it, and Priya brings in Claude Code to review it.
export const roomScene: Scene<Room> = {
  length: 11500,
  start: {
    draft: "",
    sent: false,
    working: false,
    replied: false,
    followUp: false,
  },
  steps: [
    { at: 0, cursor: "talk" },
    { at: 500, cursor: "draft", label: "Message the room" },
    { at: 1300, click: true },
    { at: 1500, type: { key: "draft", text: ask, perChar: 38 } },
    { at: 3300, cursor: "send", label: "Send" },
    { at: 4100, click: true, set: { draft: "", sent: true } },
    { at: 4700, set: { working: true }, cursor: "talk" },
    { at: 7000, set: { working: false, replied: true } },
    { at: 9000, set: { followUp: true } },
  ],
}

export function RoomScene({ frame }: { frame: Frame<Room> }) {
  const room = frame.state
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
      </PageHeader>
      <div
        className="flex min-h-0 flex-1 flex-col justify-end gap-5 overflow-hidden px-8 pb-4 group-data-[compact=true]:px-4"
        data-demo-target="talk"
      >
        <Message who="Sam" face={people.sam.face} note="9:30">
          Morning! Thursday’s release review is in the calendar.
        </Message>
        <Message who="Jordan" face={people.jordan.face} note="9:36">
          Notes from this week’s customer calls are in notes/calls.md.
        </Message>
        <Message
          who={agents.claude.name}
          face={agents.claude.mark}
          agent
          note="12s · $0.01"
        >
          I read them. Three customers asked about the billing fix.
        </Message>
        <Message who="Maya" face={people.maya.face} note="9:39">
          Can the billing fix go out in this release too?
        </Message>
        <Message who="Priya" face={people.priya.face} note="9:41">
          The release is in two weeks. Can we get a plan in here?
        </Message>
        {room.sent && (
          <Message who="Sam" face={people.sam.face} note="9:42">
            <span>
              <Mention>@Hermes</Mention> draft a plan for the next release
            </span>
          </Message>
        )}
        {room.working && (
          <p className="m-0 flex items-center gap-2 pl-11 text-xs text-muted-foreground">
            <span className="size-3 animate-spin rounded-full border-2 border-input border-t-foreground" />
            Hermes is reading roadmap.md
          </p>
        )}
        {room.replied && (
          <Message
            who={agents.hermes.name}
            face={agents.hermes.mark}
            agent
            note="41s · $0.02"
          >
            <span>
              Here’s a plan with three milestones and the risks I found.
            </span>
            <span className="flex w-fit items-center gap-2 rounded-lg border bg-surface px-3 py-1.5 text-xs font-medium">
              <FileTextIcon className="size-4 text-subtle" strokeWidth={1.75} />
              release-plan.md
            </span>
          </Message>
        )}
        {room.followUp && (
          <Message who="Priya" face={people.priya.face} note="9:44">
            <span>
              <Mention>@Claude Code</Mention> review it for anything we missed
            </span>
          </Message>
        )}
      </div>
      <Composer draft={room.draft} />
    </Shell>
  )
}

function Composer({ draft }: { draft: string }) {
  return (
    <div className="mx-6 mb-4 flex h-16 items-center gap-3 rounded-xl border bg-surface px-4 group-data-[compact=true]:mx-3">
      <span
        data-demo-target="draft"
        className={cn(
          "min-w-0 flex-1 truncate text-sm",
          draft ? "text-foreground" : "text-muted-foreground"
        )}
      >
        {draft ? (
          <>
            <Mention>{draft.slice(0, 7)}</Mention>
            {draft.slice(7)}
          </>
        ) : (
          "Message the room, mention @Hermes"
        )}
      </span>
      <span
        data-demo-target="send"
        className={cn(
          "grid size-8 place-items-center rounded-full",
          draft ? "bg-solid text-on-solid" : "text-faint"
        )}
      >
        <ArrowUpIcon className="size-4" />
      </span>
    </div>
  )
}
