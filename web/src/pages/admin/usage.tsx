import { useState, type FormEvent } from "react"
import { useNavigate } from "react-router"
import { ChevronRightIcon, DownloadIcon, EllipsisIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import {
  AgentMark,
  ChoiceSelect,
  DateField,
  Loading,
  Pager,
} from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { UsageTabsFor } from "@/pages/admin/usage-tabs"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
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
import type { UsageData, UsageView } from "@/lib/types"
import { cn } from "@/lib/utils"

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
      <div className="mb-4 flex flex-wrap items-baseline justify-between gap-x-6 gap-y-2 text-sm">
        <p className="m-0 text-muted-foreground">
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
        <nav className="flex gap-3" aria-label="Which conversations">
          {view.States.map((state) => (
            <AppLink
              key={state.Label}
              href={state.Href}
              aria-current={state.Current ? "page" : undefined}
              className="text-muted-foreground no-underline hover:text-foreground aria-[current=page]:font-medium aria-[current=page]:text-foreground"
            >
              {state.Label}
              {state.Count && (
                <span className="font-normal text-muted-foreground">
                  {" "}
                  {state.Count}
                </span>
              )}
            </AppLink>
          ))}
        </nav>
      </div>

      {(view.Rows?.length || view.Filtered) && (
        <Filters key={JSON.stringify(view.Values)} view={view} base={base} />
      )}

      {view.Rows?.length ? (
        <>
          <Card className="gap-0 overflow-hidden py-0">
            <Table className="[&_td]:whitespace-nowrap">
              <TableHeader>
                <TableRow>
                  {view.Columns.map((column) => (
                    <TableHead
                      key={column.Label}
                      aria-sort={column.Sorted ? "descending" : undefined}
                      className={cn(
                        column.Num && "text-right",
                        column.Sorted && "text-foreground"
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
                      <TableCell className="max-w-40 truncate">
                        {row.Asked}
                      </TableCell>
                    )}
                    <TableCell className="max-w-56 truncate">
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
                      className="max-w-48 truncate text-muted-foreground"
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

// Filters narrow the conversations: a search, a span of days, and who and what made the calls. Each one is part of the address.
function Filters({ view, base }: { view: UsageView; base: string }) {
  const navigate = useNavigate()
  const value = (name: string) => view.Values[name]?.[0] ?? ""
  const [search, setSearch] = useState(value("q"))
  const go = (changes: Record<string, string>) => {
    const query = new URLSearchParams()
    for (const [name, values] of Object.entries(view.Values)) {
      if (["after", "q", ...Object.keys(changes)].includes(name)) continue
      for (const each of values) query.append(name, each)
    }
    for (const [name, chosen] of Object.entries({ q: search, ...changes })) {
      if (chosen) query.set(name, chosen)
    }
    const text = query.toString()
    navigate(text ? `${base}?${text}` : base)
  }
  const submit = (event: FormEvent) => {
    event.preventDefault()
    go({})
  }
  return (
    <form className="mb-4 grid gap-3 text-sm" role="search" onSubmit={submit}>
      <div className="flex flex-wrap items-center gap-3">
        <label className="min-w-56 flex-1">
          <span className="sr-only">
            Search sessions, people, agents, agent types, providers and models
          </span>
          <Input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search"
          />
        </label>
        <div className="flex flex-wrap items-center gap-2 text-muted-foreground">
          From{" "}
          <DateField
            label="Any day"
            value={value("from")}
            onChange={(day) => go({ from: day })}
          />
          To{" "}
          <DateField
            label="Any day"
            value={value("to")}
            onChange={(day) => go({ to: day })}
          />
        </div>
        <Button type="submit">Filter</Button>
        {view.Filtered && (
          <AppLink
            href={view.Clear}
            className="text-muted-foreground no-underline underline-offset-4 hover:text-foreground hover:underline"
          >
            Clear
          </AppLink>
        )}
      </div>
      <Collapsible defaultOpen={view.Filtered}>
        <CollapsibleTrigger className="group flex w-fit cursor-pointer items-center gap-2 border-0 bg-transparent p-0 text-muted-foreground hover:text-foreground">
          <ChevronRightIcon className="size-4 transition-transform group-data-[panel-open]:rotate-90" />
          More filters
        </CollapsibleTrigger>
        <CollapsibleContent className="mt-3 flex flex-wrap gap-3">
          {view.Everyone && (
            <ChoiceSelect
              label="Person"
              all="Everyone"
              options={view.Facets.people}
              value={value("person")}
              onChange={(chosen) => go({ person: chosen })}
            />
          )}
          <ChoiceSelect
            label="Agent"
            all="Every agent"
            options={view.Facets.agents}
            value={value("agent")}
            onChange={(chosen) => go({ agent: chosen })}
          />
          <ChoiceSelect
            label="Agent type"
            all="Every agent type"
            options={view.Facets.agentTypes}
            value={value("agent_type")}
            onChange={(chosen) => go({ agent_type: chosen })}
          />
          <ChoiceSelect
            label="Provider"
            all="Every provider"
            options={view.Facets.providers}
            value={value("provider")}
            onChange={(chosen) => go({ provider: chosen })}
          />
          <ChoiceSelect
            label="Model"
            all="Every model"
            options={view.Facets.models}
            value={value("model")}
            onChange={(chosen) => go({ model: chosen })}
          />
        </CollapsibleContent>
      </Collapsible>
    </form>
  )
}
