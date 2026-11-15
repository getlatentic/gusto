import { MessageSquareIcon, UsersIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { AgentMark } from "@/components/controls"
import type { RailChat } from "@/domain/side-list"
import { ago } from "@/lib/format"
import { cn } from "@/lib/utils"

export const sideItem =
  "flex h-9 w-full min-w-0 items-center gap-2 rounded-md px-3 text-sm text-muted-foreground no-underline hover:bg-hover hover:text-foreground aria-[current=page]:bg-surface aria-[current=page]:font-medium aria-[current=page]:text-foreground aria-[current=page]:shadow-sm [&>svg]:size-4 [&>svg]:flex-none [&>svg]:text-subtle"

// SideRow is one conversation in the side list: what it is, its name, and how much is unread in it or when it last moved.
export function SideRow({
  chat,
  here,
  onGo,
}: {
  chat: RailChat
  here: string
  onGo?: () => void
}) {
  const unread = chat.Unread.Count
  return (
    <AppLink
      href={chat.Where}
      data-testid="side-row"
      className={sideItem}
      aria-current={chat.Where === here ? "page" : undefined}
      title={chat.Name}
      onClick={onGo}
    >
      {chat.Harness ? (
        <span
          className="grid size-5 flex-none place-items-center overflow-hidden rounded-full bg-sunken"
          aria-hidden="true"
        >
          <AgentMark
            mark={chat.Mark}
            markDark={chat.MarkDark}
            name={chat.Name}
          />
        </span>
      ) : chat.Room ? (
        <UsersIcon aria-hidden="true" />
      ) : (
        <MessageSquareIcon aria-hidden="true" />
      )}
      <span
        className={cn(
          "min-w-0 flex-1 truncate",
          unread > 0 && "font-medium text-foreground"
        )}
      >
        {chat.Name}
      </span>
      {unread > 0 ? (
        <span
          className="grid h-4.5 min-w-4.5 flex-none place-items-center rounded-full bg-foreground px-1.5 text-[0.6875rem] leading-none font-semibold text-background tabular-nums"
          aria-label={`${unread} unread`}
          data-testid="unread-count"
        >
          {unread}
        </span>
      ) : (
        <span className="flex-none text-xs text-subtle">{ago(chat.When)}</span>
      )}
    </AppLink>
  )
}
