import { Card } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { cn } from "@/lib/utils"
import { ModelMenu } from "./model-menu"
import { perMillion } from "./price-fields"
import { StateBadge } from "./state-badge"
import { TablePager } from "./table-pager"
import type { ModelsData, Price, Provider, ProviderModel } from "./types"

// ProviderModels are one provider's models with what each costs, and for a provider of the organization's, what can be done
// to each.
export function ProviderModels({
  provider,
  page,
}: {
  provider: Provider
  page: ModelsData
}) {
  const models = page.Models ?? []
  return (
    <Card className="@container gap-0 overflow-hidden py-0">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Model</TableHead>
            <PriceHead label="Input" />
            <PriceHead label="Output" />
            <PriceHead label="Cached input" className="@max-lg:hidden" />
            {!provider.Operator && (
              <TableHead className="w-10">
                <span className="sr-only">Actions</span>
              </TableHead>
            )}
          </TableRow>
        </TableHeader>
        <TableBody>
          {models.map((model) => (
            <ModelLine key={model.Name} provider={provider} model={model} />
          ))}
          {models.length === 0 && (
            <TableRow>
              <TableCell className="text-muted-foreground" colSpan={5}>
                No models yet.
                {!provider.Operator &&
                  ` Add the models ${provider.Name} answers.`}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
      <TablePager pages={page.Pages} />
    </Card>
  )
}

function PriceHead({
  label,
  className,
}: {
  label: string
  className?: string
}) {
  return (
    <TableHead className={cn("text-right", className)}>
      {label}{" "}
      <span className="font-normal text-muted-foreground @max-xl:hidden">
        $ / million
      </span>
    </TableHead>
  )
}

function ModelLine({
  provider,
  model,
}: {
  provider: Provider
  model: ProviderModel
}) {
  return (
    <TableRow>
      <TableCell className="w-full max-w-0 whitespace-normal">
        <code className="mr-1.5 font-mono text-xs break-all">{model.Full}</code>
        {model.Pending && <StateBadge tone="warn">off</StateBadge>}
      </TableCell>
      <Prices price={model.Price} />
      {!provider.Operator && (
        <TableCell className="text-right">
          <ModelMenu provider={provider} model={model} />
        </TableCell>
      )}
    </TableRow>
  )
}

function Prices({ price }: { price: Price | null }) {
  if (!price) {
    return (
      <>
        <TableCell
          className="text-right whitespace-normal text-muted-foreground"
          colSpan={2}
        >
          no price, so its cost is not counted against a cap
        </TableCell>
        <TableCell className="@max-lg:hidden" />
      </>
    )
  }
  const cached = price.cachedInputPerMillion
  return (
    <>
      <TableCell className="text-right font-numeric tabular-nums">
        {perMillion(price.inputPerMillion)}
      </TableCell>
      <TableCell className="text-right font-numeric tabular-nums">
        {perMillion(price.outputPerMillion)}
      </TableCell>
      <TableCell className="text-right font-numeric tabular-nums @max-lg:hidden">
        {cached !== undefined ? (
          perMillion(cached)
        ) : (
          <span className="text-muted-foreground">as input</span>
        )}
      </TableCell>
    </>
  )
}
