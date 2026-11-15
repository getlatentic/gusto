import { Card } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { StateBadge } from "./state-badge"
import { TablePager } from "./table-pager"
import type { ModelRow, ModelsData } from "./types"

// PricesTab is every model the organization has, what it costs and who may use it.
export function PricesTab({ page }: { page: ModelsData }) {
  return (
    <>
      {page.Rows?.length ? (
        <Card className="@container gap-0 overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Model</TableHead>
                <TableHead className="@max-md:hidden">Provider</TableHead>
                <TableHead className="text-right">Input / M</TableHead>
                <TableHead className="text-right">Output / M</TableHead>
                <TableHead className="text-right @max-lg:hidden">Who</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {page.Rows.map((row) => (
                <PriceLine key={row.Full} row={row} />
              ))}
            </TableBody>
          </Table>
          <TablePager pages={page.Pages} />
        </Card>
      ) : (
        <Card className="px-4 text-sm text-muted-foreground">
          No models yet. Add a provider, then the models it answers.
        </Card>
      )}
      <p className="m-0 mt-3 text-xs text-muted-foreground">
        A model that is not here cannot be chosen. Prices are dollars for a
        million tokens; a model with no price is not counted against a budget.
      </p>
    </>
  )
}

function PriceLine({ row }: { row: ModelRow }) {
  return (
    <TableRow>
      <TableCell className="w-full max-w-0" title={row.Full}>
        <span className="flex min-w-0 items-center gap-1.5">
          <span className="truncate">{row.Name}</span>
          {row.Off && <StateBadge tone="warn">off</StateBadge>}
        </span>
      </TableCell>
      <TableCell className="whitespace-nowrap text-muted-foreground @max-md:hidden">
        {row.Provider}
      </TableCell>
      {row.Input ? (
        <>
          <TableCell className="text-right font-numeric tabular-nums">
            {row.Input}
          </TableCell>
          <TableCell className="text-right font-numeric tabular-nums">
            {row.Output}
          </TableCell>
        </>
      ) : (
        <TableCell className="text-right text-muted-foreground" colSpan={2}>
          no price
        </TableCell>
      )}
      <TableCell className="text-right text-muted-foreground @max-lg:hidden">
        {row.Who}
      </TableCell>
    </TableRow>
  )
}
