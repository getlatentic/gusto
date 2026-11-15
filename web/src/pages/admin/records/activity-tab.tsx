import { ChevronRightIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { Badge } from "@/components/ui/badge"
import { Card } from "@/components/ui/card"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { plural } from "@/lib/format"
import type { Activity, ActivityRow, Health } from "@/pages/admin/records/types"

// A sign-in with the shared password names nobody, so the row says nobody rather than that.
const sharedSignIn = "the shared sign-in"

const inlineLink =
  "font-medium text-foreground underline decoration-border underline-offset-2 hover:decoration-current"

// ActivityTab is what people did, newest first, and how to check the record without trusting this server.
export function ActivityTab({
  activity,
  health,
}: {
  activity: Activity
  health: Health
}) {
  const rows = activity.Rows ?? []
  return (
    <>
      {rows.length > 0 ? (
        <Card className="gap-0 divide-y py-0">
          {rows.map((row, i) => (
            <ActivityLine key={`${row.Full} ${i}`} row={row} />
          ))}
        </Card>
      ) : (
        <p className="m-0 text-sm text-muted-foreground">
          Nothing has been recorded yet.
        </p>
      )}
      <ServerStarts activity={activity} />
      {!health.LogBad && <CheckItYourself head={health.Head} />}
    </>
  )
}

function ActivityLine({ row }: { row: ActivityRow }) {
  return (
    <div className="min-w-0 px-4 py-3">
      <p className="m-0 text-sm break-words">
        {row.Said}
        {row.Times > 1 && (
          <Badge
            variant="secondary"
            className="ml-1.5"
            title={`since ${row.Since}`}
          >
            ×{row.Times}
          </Badge>
        )}
      </p>
      <p className="m-0 mt-0.5 text-xs break-words text-muted-foreground">
        <time title={row.Full}>{row.At}</time>
        {row.Who !== sharedSignIn && ` · ${row.Who}`}
        {row.From && (
          <>
            {" · "}
            <span className="font-mono">{row.From}</span>
          </>
        )}
      </p>
    </div>
  )
}

// ServerStarts says how many times the server starting was left out, or that it is shown, with the way to the other.
function ServerStarts({ activity }: { activity: Activity }) {
  if (activity.WithServer) {
    return (
      <p className="m-0 mt-3 text-sm text-muted-foreground">
        Showing the server starting too.{" "}
        <AppLink className={inlineLink} href="/admin/records?tab=activity">
          Show only what people did
        </AppLink>
      </p>
    )
  }
  if (!activity.LeftOut) return null
  const one = activity.LeftOut === 1
  return (
    <p className="m-0 mt-3 text-sm text-muted-foreground">
      {activity.LeftOut} {plural(activity.LeftOut, "time")} the server started{" "}
      {one ? "is" : "are"} left out.{" "}
      <AppLink
        className={inlineLink}
        href="/admin/records?tab=activity&server=1"
      >
        Show {one ? "it" : "them"}
      </AppLink>
    </p>
  )
}

function CheckItYourself({ head }: { head: string }) {
  return (
    <Collapsible className="mt-6">
      <CollapsibleTrigger className="flex cursor-pointer items-center gap-1.5 text-sm font-medium text-foreground data-[panel-open]:[&>svg]:rotate-90">
        <ChevronRightIcon className="size-4 text-muted-foreground transition-transform" />
        How to check it yourself
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-3 grid max-w-prose gap-2 text-sm text-muted-foreground">
        <p className="m-0">
          Each line names the one before it, so an edit shows, and the chain
          holds. The server can only add to this record.
        </p>
        {head && (
          <>
            <p className="m-0">
              Download it, and keep this head to learn later whether lines were
              cut off the end:
            </p>
            <pre className="m-0 overflow-x-auto rounded-sm bg-muted px-3 py-2 text-foreground">
              gusto changes verify -head {head} changes.jsonl
            </pre>
          </>
        )}
      </CollapsibleContent>
    </Collapsible>
  )
}
