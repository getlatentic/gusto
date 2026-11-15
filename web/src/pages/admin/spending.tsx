import { useNavigate } from "react-router"

import { Loading } from "@/components/controls"
import { PageBar, PageBody, PageSub } from "@/components/page"
import { UsageTabsFor } from "@/pages/admin/usage-tabs"
import { Card, CardContent } from "@/components/ui/card"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { usePageData } from "@/lib/api"
import { compact, percent, usd } from "@/lib/format"
import type { SpendingData } from "@/lib/types"
import { cn } from "@/lib/utils"

// The summary of the usage: what agents cost this month, day by day, and what was used most.
export function SpendingPage() {
  const loaded = usePageData<SpendingData>()
  const navigate = useNavigate()
  const ledger = loaded.state !== "failed" ? loaded.data?.Ledger : undefined
  const months = ledger?.Months?.length
    ? ledger.Months
    : ledger
      ? [ledger.month]
      : []
  return (
    <>
      <PageBar title="Usage">
        {ledger && (
          <Select
            value={ledger.month}
            onValueChange={(month) =>
              navigate(
                `/admin/spending?month=${encodeURIComponent(String(month))}`
              )
            }
          >
            <SelectTrigger aria-label="Month" className="min-w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="end">
              {months.map((month) => (
                <SelectItem key={month} value={month}>
                  {month}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </PageBar>
      <PageBody>
        <PageSub>What agents cost this month, day by day</PageSub>
        <UsageTabsFor current="summary" />
        <Loading loaded={loaded}>
          {(page) => <Spending ledger={page.Ledger} />}
        </Loading>
      </PageBody>
    </>
  )
}

function Spending({ ledger }: { ledger: SpendingData["Ledger"] }) {
  const total = ledger.total
  const spent = total.usd === 0 && total.unmeasured > 0 ? "—" : usd(total.usd)
  const caption =
    total.unmeasured === 0
      ? "spent this month"
      : total.calls === total.unmeasured
        ? "no price set for these models"
        : "spent this month, at least"
  return (
    <>
      <section className="mb-9 grid grid-cols-[repeat(auto-fit,minmax(min(9rem,100%),1fr))] gap-3">
        <Figure value={spent} label={caption} headline />
        <Figure
          value={String(total.calls)}
          label={
            total.uncounted
              ? `model calls, ${total.uncounted} uncounted`
              : "model calls"
          }
        />
        <Figure
          value={compact(total.inputTokens)}
          label={`tokens in, ${percent(total.cachedTokens, total.inputTokens)} cached`}
        />
        <Figure value={compact(total.outputTokens)} label="tokens out" />
        <Figure
          value={String(total.refused)}
          label="refused by a cap"
          bad={total.refused > 0}
        />
      </section>

      <Card className="mb-4">
        <CardContent>
          <h2 className="m-0 mb-4 flex items-baseline gap-2 text-sm font-semibold">
            Spend by day{" "}
            <span className="font-normal text-muted-foreground">
              {ledger.Measure === "money" ? "dollars" : "tokens"}
            </span>
          </h2>
          {ledger.Daily && (
            <>
              <div
                className="flex h-40 items-end gap-1.5"
                role="img"
                aria-label={`Spending on each day of ${ledger.month}`}
              >
                {ledger.Daily.map((day) => (
                  <Tooltip key={day.Date}>
                    <TooltipTrigger
                      render={
                        <div className="group/day flex h-full min-w-0 flex-1 flex-col justify-end" />
                      }
                    >
                      <span
                        className="block min-h-[0.1875rem] rounded-t-sm bg-brand group-hover/day:opacity-80"
                        style={{ height: `${day.Height.toFixed(1)}%` }}
                      />
                    </TooltipTrigger>
                    <TooltipContent>{day.Says}</TooltipContent>
                  </Tooltip>
                ))}
              </div>
              <div className="mt-2 flex justify-between text-xs text-muted-foreground">
                {ledger.DayLabels?.map((label) => (
                  <span key={label}>{label}</span>
                ))}
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {ledger.Tops && ledger.Tops.length > 0 && (
        <div className="mb-4 grid grid-cols-3 gap-3 max-md:grid-cols-1">
          {ledger.Tops.map((top) => (
            <Card key={top.Title}>
              <CardContent className="grid content-start gap-1.5">
                <span className="text-xs font-medium text-muted-foreground">
                  {top.Title}
                </span>
                <span className="font-heading text-xl font-semibold wrap-anywhere">
                  {top.Name}
                </span>
                <span className="text-sm text-muted-foreground">
                  {top.Says}
                </span>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
      <p className="mt-4 text-xs text-muted-foreground">
        Priced from this deployment's price list, not reconciled with invoices.
      </p>
    </>
  )
}

function Figure({
  value,
  label,
  headline,
  bad,
}: {
  value: string
  label: string
  headline?: boolean
  bad?: boolean
}) {
  return (
    <Card className="py-4">
      <CardContent className="flex flex-col-reverse justify-between gap-2 px-4">
        <span className="text-xs font-medium text-muted-foreground">
          {label}
        </span>
        <span
          className={cn(
            "font-numeric text-2xl font-semibold tabular-nums",
            headline && "text-brand-ink",
            bad && "text-bad"
          )}
        >
          {value}
        </span>
      </CardContent>
    </Card>
  )
}
