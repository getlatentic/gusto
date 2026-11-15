import { useState } from "react"
import { EllipsisIcon } from "lucide-react"

import { Loading, Pager } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { usePageData } from "@/lib/api"
import { initial, usd } from "@/lib/format"
import { cn } from "@/lib/utils"

// The organization's budgets as the server keeps them. Each person's budget stops their agents; the organization's only warns.
export interface OrgBudget {
  Editable: boolean
  Set: boolean
  Limit: number
  Passed: boolean
  Err: string
  Member: number
  MemberSet: boolean
}

export interface MemberBudget {
  Set: boolean
  Own: boolean
  Limit: number
  Spent: number
  Passed: boolean
  Who: string
}

export interface BudgetRow {
  Email: string
  Name: string
  Budget: MemberBudget
  Percent: number
}

export interface BudgetsData {
  Budget: OrgBudget
  People?: BudgetRow[]
  Pages?: { Shown: string; Previous: string; Next: string }
  Listed: boolean
  Refused: string
  Unreadable?: string
  SpendUnreadable?: string
}

// A budget as a form field holds it: dollars to the cent, or nothing for none.
const dollars = (amount: number) => amount.toFixed(2)

// What each person may spend on models each month, and what each has spent of it.
export function BudgetsPage() {
  const loaded = usePageData<BudgetsData>()
  const [changing, setChanging] = useState(false)
  const budget = loaded.state === "ready" ? loaded.data.Budget : undefined
  return (
    <>
      <PageBar title="Budgets">
        {budget && !budget.Err && (
          <Button variant="outline" onClick={() => setChanging(true)}>
            Change
          </Button>
        )}
      </PageBar>
      <PageBody>
        <PageSub>What each person may spend on models each month</PageSub>
        <Loading loaded={loaded}>{(page) => <Budgets page={page} />}</Loading>
      </PageBody>
      {budget && !budget.Err && (
        <BudgetsDialog
          budget={budget}
          open={changing}
          onOpenChange={setChanging}
        />
      )}
    </>
  )
}

function Budgets({ page }: { page: BudgetsData }) {
  return (
    <>
      <p className="m-0 mb-6 text-sm text-muted-foreground">
        <BudgetsInWords budget={page.Budget} />
      </p>
      {page.Refused && <Notice>{page.Refused}</Notice>}
      {page.SpendUnreadable && (
        <Notice>
          Spending could not be read, so none is shown. {page.SpendUnreadable}
        </Notice>
      )}
      {page.Unreadable ? (
        <Notice>
          The list of people could not be read, so none is shown.{" "}
          {page.Unreadable}
        </Notice>
      ) : page.People?.length ? (
        <Card className="@container gap-0 overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Person</TableHead>
                <TableHead>Budget</TableHead>
                <TableHead className="text-right">Spent this month</TableHead>
                <TableHead>
                  <span className="sr-only">More</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {page.People.map((row) => (
                <BudgetLine key={row.Email} row={row} />
              ))}
            </TableBody>
          </Table>
          <Pager
            newer={page.Pages?.Previous}
            older={page.Pages?.Next}
            newerLabel="Previous"
            olderLabel="Next"
            shown={page.Pages?.Shown}
          />
        </Card>
      ) : (
        page.Listed && (
          <Card className="px-4 text-sm text-muted-foreground">
            Nobody is in the organization yet.
          </Card>
        )
      )}
    </>
  )
}

function BudgetsInWords({ budget }: { budget: OrgBudget }) {
  if (budget.Err) return <>The budgets could not be read. {budget.Err}</>
  return (
    <>
      {budget.MemberSet
        ? `Each person: ${usd(budget.Member)} a month; their agents stop there.`
        : "No budget for each person, so their agents do not stop."}{" "}
      {budget.Set
        ? `Organization: ${usd(budget.Limit)} a month, which only warns.`
        : "The organization has no budget to warn at."}
    </>
  )
}

function BudgetLine({ row }: { row: BudgetRow }) {
  const budget = row.Budget
  return (
    <TableRow>
      <TableCell className="w-full max-w-0">
        <span className="flex min-w-0 items-center gap-3">
          <span
            className="grid size-8 flex-none place-items-center rounded-full border bg-muted text-xs font-semibold text-muted-foreground"
            aria-hidden="true"
          >
            {initial(row.Name)}
          </span>
          <span className="grid min-w-0">
            <span className="truncate">{row.Name}</span>
            <span className="truncate text-xs text-muted-foreground">
              {row.Email}
            </span>
          </span>
        </span>
      </TableCell>
      <TableCell className="whitespace-nowrap">
        {budget.Set ? (
          <>
            {usd(budget.Limit)}
            {budget.Own && (
              <Badge variant="secondary" className="ml-1.5">
                own
              </Badge>
            )}
          </>
        ) : (
          <span className="text-muted-foreground">no limit</span>
        )}
      </TableCell>
      <TableCell className="text-right whitespace-nowrap tabular-nums">
        <span className={cn(budget.Passed && "text-bad")}>
          {budget.Spent ? (
            usd(budget.Spent)
          ) : (
            <span className="text-muted-foreground">nothing</span>
          )}
        </span>
        {budget.Passed && (
          <span className="text-xs font-medium text-bad"> reached</span>
        )}
        {budget.Set && (
          <div
            className="mt-1.5 ml-auto h-1.5 w-28 overflow-hidden rounded-full bg-border"
            aria-hidden="true"
          >
            <div
              className={cn(
                "h-full rounded-full",
                budget.Passed ? "bg-bad" : "bg-primary"
              )}
              style={{ width: `${row.Percent}%` }}
            />
          </div>
        )}
      </TableCell>
      <TableCell className="w-10 text-right">
        <OwnBudget row={row} />
      </TableCell>
    </TableRow>
  )
}

// OwnBudget is one person's own budget, behind ⋯ on their row. Left empty, they have everyone's again.
function OwnBudget({ row }: { row: BudgetRow }) {
  const budget = row.Budget
  const field = `own-${row.Email}`
  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant="outline"
            size="icon-sm"
            aria-label={`More for ${row.Name}`}
            title="More"
          />
        }
      >
        <EllipsisIcon />
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72">
        <form
          method="post"
          action="/admin/member-budget"
          className="grid gap-2"
        >
          <input type="hidden" name="who" value={budget.Who} />
          <Label htmlFor={field}>Their own budget, a month</Label>
          <span className="flex items-center gap-2">
            <Input
              id={field}
              name="usd"
              type="number"
              min="0"
              step="0.01"
              className="h-8"
              placeholder={
                budget.Set && !budget.Own
                  ? `${dollars(budget.Limit)}, as everyone`
                  : "no limit"
              }
              defaultValue={budget.Own ? dollars(budget.Limit) : undefined}
            />
            <Button size="sm" type="submit">
              Save
            </Button>
          </span>
          <p className="m-0 text-xs text-muted-foreground">
            Leave it empty to use everyone&apos;s budget again.
          </p>
        </form>
      </PopoverContent>
    </Popover>
  )
}

// BudgetsDialog sets both budgets in one form: each person's, and the organization's.
function BudgetsDialog({
  budget,
  open,
  onOpenChange,
}: {
  budget: OrgBudget
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form method="post" action="/admin/budget" className="grid gap-4">
          <DialogHeader>
            <DialogTitle>Budgets</DialogTitle>
            <DialogDescription>
              What may be spent on models in a month.
            </DialogDescription>
          </DialogHeader>
          <div className="grid grid-cols-2 gap-3 max-sm:grid-cols-1">
            <div className="grid gap-2">
              <Label htmlFor="budget-each">Each person, a month</Label>
              <Input
                id="budget-each"
                name="each"
                type="number"
                min="0"
                step="0.01"
                placeholder="No budget"
                defaultValue={
                  budget.MemberSet ? dollars(budget.Member) : undefined
                }
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="budget-org">Organization, a month</Label>
              <Input
                id="budget-org"
                name="usd"
                type="number"
                min="0"
                step="0.01"
                placeholder="No budget"
                defaultValue={budget.Set ? dollars(budget.Limit) : undefined}
              />
            </div>
          </div>
          <p className="m-0 text-sm text-muted-foreground">
            Past their own budget, a person&apos;s agents stop until next month
            or until you raise it. Give one person a different budget from the ⋯
            on their row. The organization&apos;s budget stops nothing: this
            console says when it is passed.
          </p>
          <DialogFooter>
            <DialogClose render={<Button variant="outline" type="button" />}>
              Cancel
            </DialogClose>
            <Button type="submit">Save</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
