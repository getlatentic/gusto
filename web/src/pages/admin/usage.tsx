import { DownloadIcon, EllipsisIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { AgentMark, Loading, Pager } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { UsageTabsFor } from "@/pages/admin/usage-tabs"
import { StateToggle, UsageToolbar } from "@/pages/admin/usage-toolbar"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { usePageData } from "@/lib/api"
import type { UsageData, UsageView } from "@/lib/types"
import { cn } from "@/lib/utils"

const askedBy = "Asked by"
const agentOwner = "Agent · owner"

// The columns a narrow table leaves out, the least needed first, by the width of the table's own card: the model under 56rem, who
// asked under 48rem, and whose agent under 36rem. The conversation, when, and what it cost always stay.
const narrowHides: Record<string, string> = {
  Model: "@max-4xl:hidden",
  [askedBy]: "@max-3xl:hidden",
  [agentOwner]: "@max-xl:hidden",
}

// Every conversation, what it cost, and whether it worked: a page of the usage index, narrowed and sorted by its address.
export function UsagePage() {
  const loaded = usePageData<UsageData>()
  const view = loaded.state !== "failed" ? loaded.data?.Usage : undefined
  return (
    <>
      <PageBar title="Usage">
        {view && (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button variant="outline" size="icon" aria-label="More" />
              }
            >
              <EllipsisIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem render={<a href={view.Export} />}>
                <DownloadIcon /> Download CSV
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </PageBar>
      <PageBody>
        <PageSub>
          Every conversation, what it cost, and whether it worked
        </PageSub>
        <UsageTabsFor current="tasks" />
        <Loading loaded={loaded}>
          {(page) =>
            page.Usage ? (
              <Usage view={page.Usage} base={page.Base} />
            ) : (
              <Empty>
                <EmptyDescription>
                  This server keeps no usage index.
                </EmptyDescription>
              </Empty>
            )
          }
        </Loading>
      </PageBody>
    </>
  )
}

function Usage({ view, base }: { view: UsageView; base: string }) {
  const { Figures: figures } = view
  const priced = figures.Spent !== "no price" && figures.Spent !== "—"
  return (
    <>
      {view.Building.building && (
        <Notice tone="warn">
          The index is still being built, {view.Building.done} of{" "}
          {view.Building.total} tasks. What is below is not yet everything.
        </Notice>
      )}
      {(view.Rows?.length || view.Filtered || view.Values.state?.length) && (
        <UsageToolbar
          key={JSON.stringify(view.Values)}
          view={view}
          base={base}
        />
      )}
      <div className="mb-3 flex flex-wrap items-center justify-between gap-x-6 gap-y-2">
        <p className="m-0 text-sm text-muted-foreground">
          {priced && (
            <strong className="font-semibold text-foreground">
              {figures.Spent}{" "}
            </strong>
          )}
          {figures.SpentCaption} · {figures.Calls} model calls · {figures.In}{" "}
          tokens in, {figures.Cache} cached · {figures.Out} out
          {figures.Refused > 0 && (
            <span className="text-bad">
              {" "}
              · {figures.Refused} refused by a cap
            </span>
          )}
        </p>
        <StateToggle states={view.States} />
      </div>

      {view.Rows?.length ? (
        <>
          <Card className="@container gap-0 overflow-hidden py-0">
            <Table className="[&_td]:whitespace-nowrap">
              <TableHeader>
                <TableRow>
                  {view.Columns.map((column) => (
                    <TableHead
                      key={column.Label}
                      aria-sort={column.Sorted ? "descending" : undefined}
                      className={cn(
                        column.Num && "text-right",
                        column.Sorted && "text-foreground",
                        narrowHides[column.Label]
                      )}
                    >
                      {column.Href ? (
                        <AppLink
                          href={column.Href}
                          className="text-inherit no-underline hover:text-foreground"
                        >
                          {column.Label}
                        </AppLink>
                      ) : (
                        column.Label
                      )}
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {view.Rows.map((row) => (
                  <TableRow key={row.ID}>
                    <TableCell title={row.LastFull}>{row.Last}</TableCell>
                    <TableCell className="w-full max-w-0">
                      <span className="flex min-w-0 items-baseline gap-1.5">
                        <AppLink
                          href={row.Href}
                          title={row.ID}
                          className="min-w-0 truncate no-underline underline-offset-4 hover:underline"
                        >
                          {row.Title || row.ID}
                        </AppLink>
                        {row.Turns > 1 && (
                          <span className="shrink-0 text-xs text-muted-foreground">
                            · {row.Turns} turns
                          </span>
                        )}
                      </span>
                    </TableCell>
                    {view.ShowWho && (
                      <TableCell
                        className={cn(
                          "max-w-40 truncate",
                          narrowHides[askedBy]
                        )}
                      >
                        {row.Asked}
                      </TableCell>
                    )}
                    <TableCell
                      className={cn(
                        "max-w-48 truncate",
                        narrowHides[agentOwner]
                      )}
                    >
                      <span
                        className="flex min-w-0 items-center gap-2"
                        title={row.Engine || undefined}
                      >
                        {row.Engine && (
                          <AgentMark
                            mark={row.Mark}
                            markDark={row.MarkDark}
                            name={row.Engine}
                          />
                        )}
                        <span className="truncate">
                          {row.Agent || row.Engine || "—"}{" "}
                          <span className="text-muted-foreground">
                            · {row.Owner}'s
                          </span>
                        </span>
                      </span>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {row.Cost === "no price" ? (
                        <span className="text-muted-foreground">no price</span>
                      ) : (
                        row.Cost
                      )}
                      {row.Unpriced && (
                        <span className="text-muted-foreground">
                          {" "}
                          {row.Unpriced}
                        </span>
                      )}
                    </TableCell>
                    <TableCell
                      className={cn(
                        "max-w-40 truncate text-muted-foreground",
                        narrowHides.Model
                      )}
                      title={row.Models}
                    >
                      {row.Models}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <Pager newer={view.Newest} older={view.Next} />
          </Card>
          {figures.Unpriced && (
            <p className="mt-3 text-sm text-muted-foreground">
              {figures.Unpriced}
            </p>
          )}
        </>
      ) : (
        <Nothing view={view} />
      )}
    </>
  )
}

function Nothing({ view }: { view: UsageView }) {
  let title = "No model calls yet"
  let words: React.ReactNode =
    "Every call an agent makes goes through the proxy and is listed here, whoever made it."
  if (view.Filtered) {
    title = "No session matches these filters"
    words = (
      <AppLink
        href={view.Clear}
        className="underline decoration-input underline-offset-4 hover:text-foreground"
      >
        Clear them
      </AppLink>
    )
  } else if (view.Values.state?.length) {
    title = "No conversation has failed"
    words = "One is listed here when its latest turn fails."
  } else if (view.Building.building) {
    title = "Nothing is listed yet"
    words = "The index is still being built."
  }
  return (
    <Empty className="border border-dashed">
      <EmptyHeader>
        <EmptyTitle>{title}</EmptyTitle>
        <EmptyDescription>{words}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}
