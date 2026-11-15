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
      <Card className="gap-0 overflow-hidden py-0">
        <Table className="[&_td]:whitespace-nowrap">
          <TableHeader>
            <TableRow>
              <TableHead>Time (UTC)</TableHead>
              <TableHead>Task</TableHead>
              <TableHead>Model</TableHead>
              <TableHead>Provider</TableHead>
              <TableHead>Key</TableHead>
              <TableHead className="text-right">Tokens</TableHead>
              <TableHead className="text-right">Cache</TableHead>
              <TableHead className="text-right">Cost</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Refused by</TableHead>
              <TableHead>Translated</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {session.Calls.map((call, index) => (
              <TableRow key={`${call.Task}-${index}`}>
                <TableCell title={call.AtFull}>{call.At}</TableCell>
                <TableCell className="max-w-48 truncate">
                  <a
                    href={call.Record}
                    title={`Details of ${call.Task}`}
                    className="no-underline underline-offset-4 hover:underline"
                  >
                    {call.Task}
                  </a>
                </TableCell>
                <TableCell className="max-w-48 truncate" title={call.Model}>
                  {call.Model}
                </TableCell>
                <TableCell>{call.Provider}</TableCell>
                <TableCell>{call.Key || "—"}</TableCell>
                <TableCell className="text-right tabular-nums">
                  {call.In} in, {call.Out} out
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {call.Cache}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {call.Unpriced ? (
                    <span className="text-muted-foreground">no price</span>
                  ) : (
                    call.Cost
                  )}
                </TableCell>
                <TableCell>
                  {call.Refused ? (
                    <Badge variant="destructive">
                      {call.Status || "refused"}
                    </Badge>
                  ) : (
                    call.Status || "—"
                  )}
                </TableCell>
                <TableCell>{call.Refused || "—"}</TableCell>
                <TableCell>{call.Translated || "—"}</TableCell>
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
