import { AppLink } from "@/components/app-link"
import { Card } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { ProviderPolicy } from "./provider-policy"
import { StateBadge } from "./state-badge"
import type { ModelsData, ProviderRow } from "./types"

// ProvidersTab is each provider in a line, which leads to its own addresses, key and models, and what a provider may be.
export function ProvidersTab({ page }: { page: ModelsData }) {
  return (
    <>
      {page.Providers?.length ? (
        <Card className="@container gap-0 overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Provider</TableHead>
                <TableHead className="@max-2xl:hidden">Address</TableHead>
                <TableHead className="@max-xl:hidden">Key</TableHead>
                <TableHead className="text-right @max-md:hidden">
                  Models
                </TableHead>
                <TableHead className="text-right">State</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {page.Providers.map((row) => (
                <ProviderLine key={row.Name} row={row} />
              ))}
            </TableBody>
          </Table>
        </Card>
      ) : (
        <Card className="px-4 text-sm text-muted-foreground">
          No providers yet. Add a provider, then the models it answers.
        </Card>
      )}
      {page.View.Unnamed && (
        <p className="m-0 mt-3 max-w-prose text-xs text-muted-foreground">
          This deployment&apos;s own models are reached through one provider
          with no name, so a provider cannot be added beside it. Its operator
          names it to allow that.
        </p>
      )}
      <ProviderPolicy view={page.View} sources={page.Sources ?? []} />
    </>
  )
}

function ProviderLine({ row }: { row: ProviderRow }) {
  return (
    <TableRow>
      <TableCell className="w-full max-w-0">
        <span className="flex min-w-0 flex-wrap items-baseline gap-x-1.5">
          <AppLink
            href={row.Href}
            className="truncate font-medium text-foreground no-underline hover:underline"
          >
            {row.Name}
          </AppLink>
          {row.Operator && (
            <span className="text-xs text-muted-foreground">
              set by the operator
            </span>
          )}
        </span>
      </TableCell>
      <TableCell className="whitespace-nowrap text-muted-foreground @max-2xl:hidden">
        <code className="font-mono text-xs">{row.Address}</code>
        {row.More > 0 && (
          <span className="text-faint"> and {row.More} more</span>
        )}
      </TableCell>
      <TableCell className="whitespace-nowrap text-muted-foreground @max-xl:hidden">
        {row.Key}
      </TableCell>
      <TableCell className="text-right font-numeric tabular-nums @max-md:hidden">
        {row.Models}
      </TableCell>
      <TableCell className="text-right">
        <ProviderState state={row.State} />
      </TableCell>
    </TableRow>
  )
}

function ProviderState({ state }: { state: string }) {
  if (state === "on") return <StateBadge tone="ok">on</StateBadge>
  if (state === "off") {
    return (
      <StateBadge tone="warn">off: waiting for you to turn it on</StateBadge>
    )
  }
  return <StateBadge tone="bad">{state}</StateBadge>
}
