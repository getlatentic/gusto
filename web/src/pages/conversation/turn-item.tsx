import { InfoIcon, UserIcon } from "lucide-react"
import type { ReactNode } from "react"

import { AgentMark } from "@/components/controls"
import { ownerWords } from "@/domain/live-room"
import { clock, initial } from "@/lib/format"
import { cn } from "@/lib/utils"
import { useConversation } from "@/pages/conversation/conversation-context"
import { fileHref, FileChips } from "@/pages/conversation/file-chips"
import { MessageTools } from "@/pages/conversation/message-tools"
import { ReplyResult } from "@/pages/conversation/reply-result"
import type { TurnView } from "@/pages/conversation/types"
import { withBase } from "@/lib/base"

const block = "group/msg relative grid grid-cols-[2rem_minmax(0,1fr)] gap-x-3"
const label =
  "m-0 mt-1 mb-1.5 flex h-4 min-w-0 items-center gap-2 text-xs font-medium"
const badge =
  "inline-flex h-4 flex-none items-center rounded-full bg-muted px-1.5 text-xs leading-4 font-medium text-subtle"
const shown =
  "transition-opacity [@media(hover:hover)]:opacity-0 group-focus-within/msg:opacity-100 group-hover/msg:opacity-100"
const mentions = "[&_.mention]:font-medium [&_.mention]:text-brand-ink"

// meta is when a message was sent and, for a finished reply, how long it took and what it cost.
function meta(turn: TurnView, reply: boolean): string {
  const parts = [clock(turn.Queued)]
  if (reply && turn.Done) parts.push(turn.Took, turn.Money)
  return parts.filter(Boolean).join(" · ")
}

// TurnItem is one turn of a conversation: what was said, and the agent's reply to it; or a note or notice of a room.
// live, when it is given, is what a reply still being written has done so far, drawn in place of the plain word that it works.
export function TurnItem({
  turn,
  progress,
  live,
}: {
  turn: TurnView
  progress?: string
  live?: ReactNode
}) {
  return (
    <article
      id={turn.ID}
      data-testid="turn"
      data-pending={turn.Kind === "turn" && !turn.Done ? "" : undefined}
    >
      {turn.Divider && (
        <p className="my-6 flex items-center gap-3 text-xs font-medium text-subtle before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border">
          {turn.Divider}
        </p>
      )}
      {turn.Kind === "notice" ? (
        <Notice turn={turn} />
      ) : (
        <>
          {turn.ShowSaid && <Said turn={turn} />}
          {turn.Kind === "turn" && (
            <Reply turn={turn} progress={progress} live={live} />
          )}
        </>
      )}
    </article>
  )
}

function Notice({ turn }: { turn: TurnView }) {
  const { page, openFile } = useConversation()
  const href = turn.File ? fileHref(page.Instance.ID, turn.File, page.Path) : ""
  return (
    <p
      role="status"
      className="mx-auto mt-5 flex w-fit max-w-160 items-start gap-2 rounded-lg bg-warn-wash px-3 py-2 text-sm leading-6 text-warn [&>svg]:mt-1 [&>svg]:size-4 [&>svg]:flex-none"
    >
      <InfoIcon aria-hidden="true" />
      <span>
        {turn.Said}
        {href && (
          <>
            {" "}
            <a
              className="font-medium text-inherit"
              href={withBase(href)}
              onClick={(event) =>
                openFile(href, turn.File) && event.preventDefault()
              }
            >
              Open it
            </a>
          </>
        )}
      </span>
    </p>
  )
}

function Said({ turn }: { turn: TurnView }) {
  return (
    <div className={cn(block, turn.SaidJoins ? "mt-1.5" : "mt-3")} data-msg>
      <MessageTools copy={() => turn.Said} answer={turn.WhoName} />
      {turn.SaidJoins ? (
        <time
          className={cn(
            "text-right text-xs leading-5.5 text-subtle tabular-nums",
            shown
          )}
        >
          {clock(turn.Queued)}
        </time>
      ) : (
        <>
          <span
            className="row-span-2 grid size-8 place-items-center rounded-full bg-sunken text-xs font-medium text-muted-foreground [&>svg]:size-4"
            aria-hidden="true"
          >
            {turn.WhoInitials || <UserIcon />}
          </span>
          <p className={label}>
            <span className="truncate">{turn.WhoName}</span>
            <span
              className={cn(
                "flex-none font-normal text-subtle tabular-nums",
                shown
              )}
            >
              {meta(turn, false)}
            </span>
          </p>
        </>
      )}
      <p
        className={cn(
          "col-start-2 m-0 max-w-full text-sm wrap-anywhere whitespace-pre-wrap",
          mentions
        )}
        data-testid="said"
        data-msg-body
        // The server escapes the message and marks only its mentions.
        dangerouslySetInnerHTML={{
          __html: turn.SaidHTML || escapeText(turn.Said),
        }}
      />
    </div>
  )
}

const escapeText = (text: string) =>
  text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")

function Reply({
  turn,
  progress,
  live,
}: {
  turn: TurnView
  progress?: string
  live?: ReactNode
}) {
  const { page, admin, openFile } = useConversation()
  return (
    <div
      className={cn(block, turn.ReplyJoins ? "mt-1.5" : "mt-3")}
      data-msg
      data-testid="reply"
      data-done={turn.Done ? "" : undefined}
    >
      {turn.Done && (
        <MessageTools copy={() => textOf(turn.ReplyHTML)} answer={turn.By} />
      )}
      {turn.ReplyJoins ? (
        <time
          className={cn(
            "text-right text-xs leading-5.5 text-subtle tabular-nums",
            shown
          )}
          title={meta(turn, true)}
        >
          {clock(turn.Queued)}
        </time>
      ) : (
        <>
          {turn.Mark ? (
            <span className="row-span-2 grid size-8" aria-hidden="true">
              <AgentMark
                mark={turn.Mark}
                markDark={turn.MarkDark}
                name={turn.By}
                size="size-8"
              />
            </span>
          ) : (
            <span
              className="row-span-2 grid size-8 place-items-center rounded-full bg-sunken text-xs font-medium text-muted-foreground"
              aria-hidden="true"
            >
              {initial(turn.By)}
            </span>
          )}
          <p className={label}>
            <span className="truncate" title={turn.Role || undefined}>
              {turn.By}
            </span>
            <span className={badge}>{ownerWords(turn, page.Me)}</span>
            {turn.From && (
              <span className="truncate font-normal text-subtle">
                asked by @{turn.From}
              </span>
            )}
            {turn.Done && (
              <span
                className={cn(
                  "flex-none font-normal text-subtle tabular-nums",
                  shown
                )}
              >
                {meta(turn, true)}
              </span>
            )}
          </p>
        </>
      )}
      <div className="col-start-2 grid min-w-0 gap-3 text-sm">
        {turn.Done ? (
          <ReplyResult turn={turn} page={page} admin={admin} />
        ) : (
          (live ?? <Working turn={turn} progress={progress} />)
        )}
      </div>
      {turn.Done && turn.Files && (
        <FileChips
          files={turn.Files}
          instance={page.Instance.ID}
          from={page.Path}
          onOpen={openFile}
        />
      )}
    </div>
  )
}

// textOf is the words of a reply's HTML, for copying.
function textOf(html: string): string {
  return (
    new DOMParser().parseFromString(html, "text/html").body.textContent ?? ""
  )
}

// Working is a reply still being written: why it waits, or what it is doing now, and the way to stop it.
function Working({ turn, progress }: { turn: TurnView; progress?: string }) {
  return (
    <div className="flex items-center gap-2 text-sm text-muted-foreground">
      {turn.Holding ? (
        <>
          <span
            className="size-3 flex-none rounded-full border-2 border-input"
            aria-hidden="true"
          />
          <span>{turn.Holding}</span>
        </>
      ) : (
        <>
          <span
            className="size-3 flex-none animate-spin rounded-full border-2 border-input border-t-foreground"
            aria-hidden="true"
          />
          <span className="min-w-0 truncate" role="status">
            {progress || "Working"}
          </span>
        </>
      )}
      <StopButton job={turn.ID} />
    </div>
  )
}

// StopButton stops the job writing a reply.
export function StopButton({ job }: { job: string }) {
  const { page, stop } = useConversation()
  return (
    <form
      method="post"
      action={withBase(`/jobs/${encodeURIComponent(job)}/stop`)}
      className="m-0 ml-2 flex-none"
      onSubmit={(event) => {
        event.preventDefault()
        stop(job, event.currentTarget)
      }}
    >
      <input type="hidden" name="back" value={page.Path} />
      <button
        type="submit"
        className="cursor-pointer border-0 bg-transparent p-0 text-xs text-subtle underline-offset-2 hover:text-foreground hover:underline"
      >
        Stop
      </button>
    </form>
  )
}
