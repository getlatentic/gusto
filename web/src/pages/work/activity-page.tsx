import { AppLink } from "@/components/app-link"
import { Loading, LinkTabs } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { buttonVariants } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { everythingHref, whyEmpty } from "@/domain/work"
import { usePageData } from "@/lib/api"
import { ActivityFilters } from "@/pages/work/activity-filters"
import { RunLine } from "@/pages/work/run-line"
import type { ActivityData, Board } from "@/pages/work/types"

// ActivityPage is every task agents ran that the person may read, newest first, a page at a time, with what is going on now
// counted across the top as the filters it stands for. It starts nothing itself; a run happens on an agent.
export function ActivityPage() {
  const loaded = usePageData<ActivityData>()
  return (
    <>
      <PageBar title="Activity" />
      <PageBody className="max-w-5xl">
        <Loading loaded={loaded}>
          {(page) => <ActivityBoard board={page.Board} />}
        </Loading>
      </PageBody>
    </>
  )
}

function ActivityBoard({ board }: { board: Board }) {
  const days = board.Days ?? []
  const building = board.Index?.Building
  return (
    <>
      <PageSub>
        Every task agents ran, newest first{board.Shown && ` · ${board.Shown}`}
      </PageSub>
      {building?.building && (
        <Notice tone="warn">
          The index is still being built, {building.done} of {building.total}{" "}
          tasks. What is below is not yet everything.
        </Notice>
      )}
      <StateTabs board={board} />
      <ActivityFilters board={board} />
      {days.length > 0 ? (
        <>
          {days.map((day, at) => (
            <section
              key={`${day.Named}-${at}`}
              className="mb-6"
              aria-label={day.Named || undefined}
            >
              <Card className="gap-0 py-0">
                {day.Named && (
                  <h2 className="m-0 border-b bg-muted/50 px-5 py-3 text-xs font-medium text-muted-foreground">
                    {day.Named}
                  </h2>
                )}
                <ol className="m-0 list-none divide-y p-0">
                  {day.Runs.map((run) => (
                    <RunLine key={run.ID} run={run} />
                  ))}
                </ol>
              </Card>
            </section>
          ))}
          <Pages board={board} />
        </>
      ) : (
        <Empty board={board} />
      )}
    </>
  )
}

// StateTabs are the counts across the top: everything, then each state that has some or is chosen.
function StateTabs({ board }: { board: Board }) {
  const figures = (board.Figures ?? []).filter(
    (figure) => figure.Count > 0 || figure.Current
  )
  const tabs = [
    {
      key: "everything",
      label: "Everything",
      href: everythingHref(board.Harness),
    },
    ...figures.map((figure) => ({
      key: figure.Label,
      href: figure.Href,
      label: (
        <span className="capitalize" data-testid={`figure-${figure.Label}`}>
          {figure.Label}{" "}
          <span className="font-normal text-subtle tabular-nums">
            {figure.Count}
          </span>
        </span>
      ),
    })),
  ]
  return (
    <LinkTabs
      tabs={tabs}
      current={board.State || "everything"}
      label="What happened, this week"
    />
  )
}

function Pages({ board }: { board: Board }) {
  const links = [
    { label: "Newest", href: board.Index?.Newest ?? "" },
    { label: "Newer", href: board.Newer },
    { label: "Older", href: board.Older },
  ].filter((link) => link.href)
  if (links.length === 0) return null
  return (
    <nav className="mt-4 flex justify-end gap-2" aria-label="Pages">
      {links.map((link) => (
        <AppLink
          key={link.label}
          href={link.href}
          className={buttonVariants({ variant: "outline", size: "sm" })}
        >
          {link.label}
        </AppLink>
      ))}
    </nav>
  )
}

function Empty({ board }: { board: Board }) {
  const why = whyEmpty(board)
  return (
    <Card className="px-6 py-12 text-center text-muted-foreground">
      <p className="m-0" data-testid="activity-empty">
        {why === "filtered" && "Nothing matches these filters."}
        {why === "building" &&
          "Nothing is listed yet: the index is still being built."}
        {why === "nothing" && (
          <>
            Nothing has happened here yet. Everything an agent does shows up
            here:{" "}
            <a className="font-medium text-foreground" href="/presets">
              start one
            </a>
            .
          </>
        )}
      </p>
    </Card>
  )
}
