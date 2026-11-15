import { useLayoutEffect, useRef } from "react"

import { Notice } from "@/components/page"
import { pending, typingWords } from "@/domain/live-room"
import { RunningReply } from "@/pages/conversation/running-reply"
import { TurnItem } from "@/pages/conversation/turn-item"
import type { ConversationData, TurnView } from "@/pages/conversation/types"
import { useProgress } from "@/pages/conversation/use-progress"

// The page keeps following the end of the conversation while the person is within this many pixels of it.
const followWithin = 96

// TurnList is the conversation so far, newest last. It opens at the end, and stays there as turns come in, unless the person
// has scrolled up to read.
export function TurnList({
  page,
  turns,
  typing,
  failed,
  onStarter,
}: {
  page: ConversationData
  turns: TurnView[]
  typing: string[]
  failed: string
  onStarter: (said: string) => void
}) {
  const region = useRef<HTMLDivElement>(null)
  const following = useRef(true)
  const progress = useProgress(page.Path, page.Instance.Room && pending(turns))
  const last = turns[turns.length - 1]
  // Keeping the end in view is the page's layout following its content, which only an effect can do.
  useLayoutEffect(() => {
    const at = region.current
    if (at && following.current) at.scrollTop = at.scrollHeight
  }, [turns.length, last?.ID, last?.Done, page.Waiting])
  const words = typingWords(typing)
  const running = !page.Instance.Room && page.Waiting
  return (
    <div
      ref={region}
      role="region"
      aria-label="Conversation"
      tabIndex={0}
      className="relative overflow-y-auto px-6 py-8 focus-visible:-outline-offset-2 max-nav:px-4 max-nav:py-6"
      onScroll={(event) => {
        const at = event.currentTarget
        following.current =
          at.scrollHeight - at.scrollTop - at.clientHeight < followWithin
      }}
    >
      <div className="mx-auto max-w-208">
        {failed && <Notice>{failed}</Notice>}
        {turns.length > 0 ? (
          <ol aria-label="Messages" className="m-0 list-none p-0">
            {turns.map((turn) => (
              <li key={turn.ID}>
                <TurnItem turn={turn} progress={progress[turn.ID]} />
              </li>
            ))}
          </ol>
        ) : (
          !running && <Empty page={page} onStarter={onStarter} />
        )}
        {running && <RunningReply page={page} />}
        {page.Stream && (
          <p className="m-0 mt-2 h-5 text-xs text-subtle" aria-hidden="true">
            {words}
          </p>
        )}
        <p className="sr-only" role="status">
          {words}
        </p>
      </div>
    </div>
  )
}

// Empty is a conversation with nothing said in it: what it is for, and a few things to start with.
function Empty({
  page,
  onStarter,
}: {
  page: ConversationData
  onStarter: (said: string) => void
}) {
  const direct = page.DirectAgent
  const heading = page.Instance.Room
    ? direct
      ? direct.Name
      : page.Instance.Title
    : "How can I help?"
  return (
    <section
      aria-label="Getting started"
      className="grid justify-items-center gap-6 pt-16 pb-8 text-center"
    >
      <h2 className="m-0 text-xl font-semibold">{heading}</h2>
      {page.Instance.Room && <RoomGreeting page={page} />}
      {(page.Starters ?? []).length > 0 && (
        <ul
          aria-label="Ways to start"
          className="m-0 grid w-full list-none grid-cols-[repeat(auto-fit,minmax(14rem,1fr))] gap-3 p-0"
        >
          {page.Starters!.map((said) => (
            <li key={said}>
              <button
                type="button"
                className="w-full cursor-pointer rounded-md border bg-surface px-4 py-3 text-left text-sm hover:border-input"
                onClick={() => onStarter(said)}
              >
                {said}
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

function RoomGreeting({ page }: { page: ConversationData }) {
  const direct = page.DirectAgent
  if (direct) {
    return (
      <p className="m-0 max-w-128 text-sm text-muted-foreground">
        Your own conversation with {direct.Name}. It answers everything you say
        here.
      </p>
    )
  }
  if ((page.Members?.RoomAgents ?? 0) > 0) {
    return (
      <p className="m-0 max-w-128 text-sm text-muted-foreground">
        Mention an agent with @ and its name to ask it something. Everybody in
        the room sees what it says.
      </p>
    )
  }
  return (
    <p className="m-0 max-w-128 text-sm text-muted-foreground">
      This room has no agents yet.{" "}
      <a
        className="font-medium text-foreground underline"
        href={`${page.Path}?add=1`}
      >
        Add one
      </a>{" "}
      to start.
    </p>
  )
}
