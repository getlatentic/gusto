import { useState, type FormEvent } from "react"
import { useNavigate, useParams } from "react-router"

import { AppLink } from "@/components/app-link"
import { Loading, Pager } from "@/components/controls"
import { PageBar, PageBody, PageSub } from "@/components/page"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
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

// The columns a narrow table leaves out, the least needed first, by the width of the table's own card. When, which task, how many
// what it cost always stay.
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

// One conversation's model calls, the latest first.
export function UsageSessionPage() {
  const loaded = usePageData<SessionData>()
  const id = useParams().session ?? ""
  return (
    <>
      <PageBar
        crumbs={[{ label: "Usage", href: "/admin/usage" }]}
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
  const [search, setSearch] = useState(session.Search)
  const submit = (event: FormEvent) => {
    event.preventDefault()
    navigate(search ? `${here}?q=${encodeURIComponent(search)}` : here)
  }
  return (
    <>
      <PageSub>
        {session.Person}
        {session.Agent && `, agent ${session.Agent}`}
        {session.Engine && `, agent type ${session.Engine}`}. The latest call
        first.
      </PageSub>
      <form
        className="mb-4 flex flex-wrap items-center gap-3 text-sm"
        role="search"
        onSubmit={submit}
      >
        <label className="min-w-48 flex-1">
          <span className="sr-only">Search this session's calls</span>
          <Input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search"
          />
        </label>
        <Button type="submit">Search</Button>
        {session.Search && (
          <AppLink
            href={here}
            className="text-muted-foreground no-underline underline-offset-4 hover:text-foreground hover:underline"
          >
            Clear
          </AppLink>
        )}
      </form>
      <Card className="@container gap-0 overflow-hidden py-0">
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
              <TableRow key={`${call.Task}-${index}`}>
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
