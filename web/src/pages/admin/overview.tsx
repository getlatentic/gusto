import { AppLink } from "@/components/app-link"
import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { Card, CardContent } from "@/components/ui/card"
import { usePageData } from "@/lib/api"
import { initial, usd } from "@/lib/format"
import type { OverviewData, SpendLine } from "@/lib/types"
import { cn } from "@/lib/utils"

// The admin's first page: what the organization has spent this month, against its budget, and who spent it.
export function OverviewPage() {
  const loaded = usePageData<OverviewData>()
  return (
    <>
      <PageBar title="Overview" />
      <PageBody>
        <Loading loaded={loaded}>{(page) => <Overview page={page} />}</Loading>
      </PageBody>
    </>
  )
}

function Overview({ page }: { page: OverviewData }) {
  const { Budget: budget, Spent: spent } = page
  return (
    <>
      <PageSub>{page.MonthName} so far</PageSub>
      {budget.Passed && (
        <Notice>
          The budget for {page.MonthName} is {usd(budget.Limit)}.{" "}
          {usd(spent.USD)} has been spent
          {spent.Unmeasured > 0 && ", and more that has no price"}, so the
          budget is{" "}
          {budget.Over
            ? `passed by ${usd(budget.Over)}${spent.Unmeasured > 0 ? " at least" : ""}`
            : "reached"}
          .
        </Notice>
      )}
      {budget.Err && (
        <Notice>
          The budget could not be read, so it is not checked. {budget.Err}
        </Notice>
      )}
      {spent.Building && (
        <Notice tone="warn">
          The index of model calls is still being built, so these figures are
          not yet everything.
        </Notice>
      )}

      <Card className="mb-9">
        <CardContent className="grid gap-1">
          <p className="m-0 text-sm text-muted-foreground">Spent this month</p>
          <p className="m-0 font-heading text-3xl font-semibold tracking-tight tabular-nums">
            {usd(spent.USD)}
            {budget.Set && (
              <span className="text-base font-normal tracking-normal text-muted-foreground">
                {" "}
                of {usd(budget.Limit)}
              </span>
            )}
          </p>
          {budget.Set ? (
            <Meter
              percent={page.Percent ?? 0}
              over={budget.Passed}
              className="mt-2"
            />
          ) : (
            <p className="m-0 text-sm text-muted-foreground">
              No budget set.{" "}
              {page.BudgetsPage && (
                <a
                  className="font-medium text-foreground underline decoration-input underline-offset-2 hover:decoration-current"
                  href="/admin/budgets"
                >
                  Set a budget
                </a>
              )}
            </p>
          )}
          {spent.Unmeasured > 0 && (
            <p className="m-0 mt-1 text-xs text-muted-foreground">
              At least: {spent.Unmeasured}{" "}
              {spent.Unmeasured === 1 ? "call has" : "calls have"} no price.
            </p>
          )}
          {page.Credit && (
            <p className="m-0 mt-1 text-xs text-muted-foreground">
              Free model credit: {usd(page.Credit.Used)} of{" "}
              {usd(page.Credit.Limit)} used.
            </p>
          )}
        </CardContent>
      </Card>

      {spent.Split && (
        <div className="grid gap-9">
          <Section
            title="By person"
            more={page.PeopleAll}
            empty="Nobody has spent anything this month."
          >
            {spent.People.map((line) => (
              <Line key={line.Label} line={line} avatar meter />
            ))}
          </Section>
          <Section
            title="By agent type"
            more={spent.AllUsage}
            empty="No agent has spent anything this month."
          >
            {spent.Engines.map((line) => (
              <Line key={line.Label} line={line} />
            ))}
          </Section>
        </div>
      )}
    </>
  )
}

function Section({
  title,
  more,
  empty,
  children,
}: {
  title: string
  more: string
  empty: string
  children: React.ReactNode[]
}) {
  return (
    <section className="grid gap-3">
      <h2 className="m-0 flex items-baseline justify-between px-1 text-sm font-semibold">
        {title}
        <AppLink
          href={more}
          className="text-sm font-medium text-muted-foreground no-underline hover:text-foreground"
        >
          View all
        </AppLink>
      </h2>
      <Card className="gap-0 overflow-hidden py-0">
        {children.length > 0 ? (
          <div className="divide-y">{children}</div>
        ) : (
          <p className="m-0 px-5 py-4 text-sm text-muted-foreground">{empty}</p>
        )}
      </Card>
    </section>
  )
}

function Line({
  line,
  avatar,
  meter,
}: {
  line: SpendLine
  avatar?: boolean
  meter?: boolean
}) {
  const body = (
    <>
      <span className="flex min-w-0 items-center gap-2.5">
        {avatar && (
          <span
            className="grid size-7 flex-none place-items-center rounded-full border bg-muted text-xs font-semibold text-muted-foreground"
            aria-hidden="true"
          >
            {line.More ? "+" : initial(line.Label)}
          </span>
        )}
        <span className="truncate">
          {line.Label}
          {line.Sub && (
            <span className="text-muted-foreground"> · {line.Sub}</span>
          )}
        </span>
      </span>
      {meter &&
        (line.Budget ? (
          <Meter
            percent={line.Percent}
            over={line.Percent >= 100}
            className="max-sm:hidden"
          />
        ) : (
          <span className="max-sm:hidden" />
        ))}
      <span className="text-right text-sm whitespace-nowrap tabular-nums">
        {!line.Calls ? (
          <span className="text-muted-foreground">Nothing yet</span>
        ) : line.Unpriced && line.USD === 0 ? (
          <span className="text-muted-foreground">No price set</span>
        ) : (
          usd(line.USD)
        )}
        {line.Budget > 0 && (
          <span className="text-muted-foreground"> of {usd(line.Budget)}</span>
        )}
      </span>
    </>
  )
  const grid = cn(
    "grid min-h-14 items-center gap-4 px-5 py-3 text-inherit no-underline",
    meter
      ? "grid-cols-[minmax(0,1fr)_7.5rem_9.5rem] max-sm:grid-cols-[minmax(0,1fr)_auto]"
      : "grid-cols-[minmax(0,1fr)_auto]"
  )
  return line.Href ? (
    <AppLink href={line.Href} className={cn(grid, "hover:bg-accent")}>
      {body}
    </AppLink>
  ) : (
    <div className={grid}>{body}</div>
  )
}

function Meter({
  percent,
  over,
  className,
}: {
  percent: number
  over: boolean
  className?: string
}) {
  return (
    <span
      className={cn(
        "block h-1.5 overflow-hidden rounded-full bg-border",
        className
      )}
      aria-hidden="true"
    >
      <span
        className={cn(
          "block h-full rounded-full",
          over ? "bg-bad" : "bg-brand"
        )}
        style={{ width: `${percent}%` }}
      />
    </span>
  )
}
