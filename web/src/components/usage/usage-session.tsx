import { Link, useNavigate, useParams } from "react-router"
import { XIcon } from "lucide-react"

import { Loading, Pager } from "@/components/controls"
import { SearchBox } from "@/components/search-box"
import { PageBar, PageBody, PageSub } from "@/components/page"
import type { UsagePlace } from "@/components/usage/usage-place"
import { Badge } from "@/components/ui/badge"
import { buttonVariants } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { usePageData } from "@/lib/api"
import type { SessionData } from "@/lib/types"
import { cn } from "@/lib/utils"

// The columns a narrow table leaves out, the least needed first, by the width of the table's own card. When, which task and what it
// cost always stay.
const hides = {
  translated: "@max-6xl:hidden",
  refused: "@max-5xl:hidden",
  key: "@max-4xl:hidden",
  provider: "@max-3xl:hidden",
  cache: "@max-2xl:hidden",
  model: "@max-xl:hidden",
  status: "@max-lg:hidden",
  tokens: "@max-md:hidden",
}

// One conversation's model calls, the latest first, under the usage it is listed in.
export function UsageSession({ place }: { place: UsagePlace }) {
  const loaded = usePageData<SessionData>()
  const id = useParams().session ?? ""
  return (
    <>
      <PageBar
        crumbs={[{ label: "Usage", href: place.conversations }]}
        title={
          <>
            Session <code className="font-mono text-xs font-medium">{id}</code>
          </>
        }
      />
      <PageBody>
        <Loading loaded={loaded}>
          {(page) => <Session key={page.Session.Search} page={page} />}
        </Loading>
      </PageBody>
    </>
  )
}

function Session({ page }: { page: SessionData }) {
  const session = page.Session
  const here = `${page.Base}/${encodeURIComponent(session.ID)}`
  const navigate = useNavigate()
  return (
    <>
      <PageSub>
        {session.Person}
        {session.Agent && `, agent ${session.Agent}`}
        {session.Engine && `, agent type ${session.Engine}`}. The latest call
        first.
      </PageSub>
      <div className="mb-4 flex flex-wrap items-center gap-2" role="search">
        <SearchBox
          label="Search this session's calls"
          initial={session.Search}
          onSearch={(q) =>
            navigate(q ? `${here}?q=${encodeURIComponent(q)}` : here)
          }
        />
        {session.Search && (
          <Link
            to={here}
            className={buttonVariants({ variant: "ghost", size: "sm" })}
          >
            Reset <XIcon />
          </Link>
        )}
      </div>
      <Card
        className="@container gap-0 overflow-hidden py-0"
        data-testid="session-calls"
      >
        <Table className="[&_td]:whitespace-nowrap">
          <TableHeader>
            <TableRow>
              <TableHead>Time (UTC)</TableHead>
              <TableHead>Task</TableHead>
              <TableHead className={hides.model}>Model</TableHead>
              <TableHead className={hides.provider}>Provider</TableHead>
              <TableHead className={hides.key}>Key</TableHead>
              <TableHead className={cn("text-right", hides.tokens)}>
                Tokens
              </TableHead>
              <TableHead className={cn("text-right", hides.cache)}>
                Cache
              </TableHead>
              <TableHead className="text-right">Cost</TableHead>
              <TableHead className={hides.status}>Status</TableHead>
              <TableHead className={hides.refused}>Refused by</TableHead>
              <TableHead className={hides.translated}>Translated</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {session.Calls.map((call, index) => (
              <TableRow
                key={`${call.Task}-${index}`}
                data-testid="session-call"
              >
                <TableCell title={call.AtFull}>{call.At}</TableCell>
                <TableCell className="w-full max-w-0 truncate">
                  <a
                    href={call.Record}
                    title={`Details of ${call.Task}`}
                    className="no-underline underline-offset-4 hover:underline"
                  >
                    {call.Task}
                  </a>
                </TableCell>
                <TableCell
                  className={cn("max-w-48 truncate", hides.model)}
                  title={call.Model}
                >
                  {call.Model}
                </TableCell>
                <TableCell className={hides.provider}>
                  {call.Provider}
                </TableCell>
                <TableCell className={hides.key}>{call.Key || "—"}</TableCell>
                <TableCell
                  className={cn("text-right tabular-nums", hides.tokens)}
                >
                  {call.In} in, {call.Out} out
                </TableCell>
                <TableCell
                  className={cn("text-right tabular-nums", hides.cache)}
                >
                  {call.Cache}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {call.Unpriced ? (
                    <span className="text-muted-foreground">no price</span>
                  ) : (
                    call.Cost
                  )}
                </TableCell>
                <TableCell className={hides.status}>
                  {call.Refused ? (
                    <Badge variant="destructive">
                      {call.Status || "refused"}
                    </Badge>
                  ) : (
                    call.Status || "—"
                  )}
                </TableCell>
                <TableCell className={hides.refused}>
                  {call.Refused || "—"}
                </TableCell>
                <TableCell className={hides.translated}>
                  {call.Translated || "—"}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {session.Calls.length === 0 && (
          <p className="m-0 px-6 py-12 text-center text-sm text-muted-foreground">
            No call of this session matches.
          </p>
        )}
        <Pager
          newer={session.Newest}
          older={session.Next}
          newerLabel="Latest"
          olderLabel="Earlier"
        />
      </Card>
    </>
  )
}
