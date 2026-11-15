import { ShieldCheckIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { plural } from "@/lib/format"
import type { ConversationData, TurnView } from "@/pages/conversation/types"

const alert = "m-0 rounded-sm bg-bad-wash px-4 py-3 text-sm text-bad"
const disclosure =
  "inline-flex min-h-6 w-fit cursor-pointer items-center gap-1 rounded-sm border-0 bg-transparent py-0.5 pr-2 pl-1 text-xs hover:bg-muted [&_svg]:size-3.5"

// ReplyResult is what a finished reply came to: the steps it took, its words, and why it stopped when it stopped early.
export function ReplyResult({
  turn,
  page,
  admin,
}: {
  turn: TurnView
  page: ConversationData
  admin: boolean
}) {
  return (
    <div className="grid min-w-0 gap-1.5">
      <Steps turn={turn} />
      {turn.ReplyHTML && (
        <div
          className="in-bubble prose min-w-0"
          data-msg-body
          // The reply's markdown is drawn and made safe by the server, which rewrites its links to the conversation's files.
          dangerouslySetInnerHTML={{ __html: turn.ReplyHTML }}
        />
      )}
      <Capped turn={turn} admin={admin} />
      <Ended turn={turn} />
      <Blocked turn={turn} />
      {page.Sharing &&
        (turn.Previews ?? []).map((preview) => (
          <form
            key={preview}
            method="post"
            action={`/instances/${page.Instance.ID}/apps`}
            className="m-0"
          >
            <input type="hidden" name="preview" value={preview} />
            <input type="hidden" name="back" value={page.Path} />
            <Button type="submit" variant="outline" size="sm">
              Preview {preview !== "." ? preview : "this page"}
            </Button>
          </form>
        ))}
    </div>
  )
}

function Steps({ turn }: { turn: TurnView }) {
  const steps = turn.Steps ?? []
  if (steps.length === 0) return null
  return (
    <Collapsible className="text-xs">
      <CollapsibleTrigger
        className={`${disclosure} text-subtle hover:text-foreground`}
      >
        {steps.length} {plural(steps.length, "step")}
      </CollapsibleTrigger>
      <CollapsibleContent>
        {turn.Apps && (
          <p className="m-0 mt-2 ml-2 text-muted-foreground">
            Could use {turn.Apps}
          </p>
        )}
        <ol className="m-0 mt-2 ml-2 grid list-none gap-2 border-l border-input pl-4">
          {steps.map((step, index) => (
            <li key={index}>
              <span className="mr-2 font-semibold">{step.What}</span>
              {step.Detail && (
                <span className="font-mono text-xs wrap-anywhere text-muted-foreground">
                  {step.Detail}
                </span>
              )}
            </li>
          ))}
        </ol>
      </CollapsibleContent>
    </Collapsible>
  )
}

// Capped is the spending limit that stopped a reply: the free credit, used up, or a limit the organization set.
function Capped({ turn, admin }: { turn: TurnView; admin: boolean }) {
  const capped = turn.Capped
  if (!capped) return null
  if (capped.Credit) {
    return (
      <p className="m-0 rounded-sm bg-warn-wash px-4 py-3 text-sm">
        The free credit for the models gusto provides is used: $
        {capped.SpentUSD.toFixed(2)} of ${capped.LimitUSD.toFixed(2)}.{" "}
        {admin ? (
          <>
            <a
              className="font-medium text-foreground underline"
              href="/admin/models"
            >
              Add your organization&apos;s own model key
            </a>{" "}
            to carry on.
          </>
        ) : (
          "Ask an admin of your organization to add its own model key to carry on."
        )}
      </p>
    )
  }
  return (
    <p className={alert}>{capped.Words || "A spending limit stopped this."}</p>
  )
}

// Ended is why a reply stopped early: someone stopped it, the agents' home was full, or it failed.
function Ended({ turn }: { turn: TurnView }) {
  if (turn.StoppedBy)
    return <p className={alert}>Stopped by {turn.StoppedBy}.</p>
  if (turn.Home) {
    return (
      <p className={alert} role="alert">
        Your agents&apos; home is full: it takes {turn.Home.Used} of the{" "}
        {turn.Home.Limit} it may, so this turn was{" "}
        {turn.Home.Outcome || "ended"}. Reset it on{" "}
        <a className="font-medium text-bad underline" href="/connections">
          Connections
        </a>{" "}
        to start again.
      </p>
    )
  }
  if (turn.Capped || !turn.Stopped) return null
  return (
    <p className={alert}>
      It stopped before it finished:{" "}
      <code className="font-mono text-xs wrap-anywhere">{turn.Stopped}</code>
    </p>
  )
}

// Blocked is what the sandbox refused the agent: it may reach its model and nothing else.
function Blocked({ turn }: { turn: TurnView }) {
  if (!turn.Blocked) return null
  return (
    <Collapsible className="text-xs text-subtle">
      <CollapsibleTrigger className={`${disclosure} text-ok`}>
        <ShieldCheckIcon aria-hidden="true" />
        {turn.Blocked} {plural(turn.Blocked, "request")} blocked
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-1 ml-2 grid gap-1 border-l border-input pl-4 text-muted-foreground">
        <p className="m-0">
          The agent may reach its model and nothing else. gusto refused these:
        </p>
        <ul className="m-0 grid list-none gap-0.5 p-0">
          {(turn.Refused ?? []).map((refused) => (
            <li key={refused.Host}>
              <code className="font-mono text-xs wrap-anywhere text-foreground">
                {refused.Host}
              </code>
              {refused.Times > 1 && <span> × {refused.Times}</span>}
            </li>
          ))}
        </ul>
      </CollapsibleContent>
    </Collapsible>
  )
}
