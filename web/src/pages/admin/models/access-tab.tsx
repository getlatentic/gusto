import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Field, FormDialog } from "@/components/form-dialog"
import { PostButton } from "./post-button"
import { TablePager } from "./table-pager"
import type { Limit, ModelsData } from "./types"

// AccessTab is the people limited to some of the models, and a way to limit another.
export function AccessTab({ page }: { page: ModelsData }) {
  const available = page.View.Available ?? []
  return (
    <>
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <p className="m-0 max-w-prose text-xs text-muted-foreground">
          Everybody may use every model unless they are limited here. A person
          who is limited is offered only the models you tick, when they start an
          agent, change its model or compare agents.
        </p>
        {available.length > 0 && (
          <LimitPerson available={available} people={page.People ?? []} />
        )}
      </div>
      {page.Limits?.length ? (
        <Card className="@container gap-0 overflow-hidden py-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Person</TableHead>
                <TableHead className="@max-lg:hidden">May use</TableHead>
                <TableHead>
                  <span className="sr-only">Actions</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {page.Limits.map((limit) => (
                <LimitLine key={limit.email} limit={limit} />
              ))}
            </TableBody>
          </Table>
          <TablePager pages={page.Pages} />
        </Card>
      ) : (
        <Card className="px-4 text-sm text-muted-foreground">
          Nobody is limited, so everybody may use every model.
        </Card>
      )}
    </>
  )
}

function LimitLine({ limit }: { limit: Limit }) {
  const models = <ModelList models={limit.models ?? []} />
  return (
    <TableRow>
      <TableCell className="w-full max-w-0">
        <span className="block truncate">{limit.email}</span>
        <span className="block text-xs whitespace-normal text-muted-foreground @lg:hidden">
          {models}
        </span>
      </TableCell>
      <TableCell className="whitespace-normal @max-lg:hidden">
        {models}
      </TableCell>
      <TableCell className="text-right">
        <PostButton
          action="/admin/models/access/clear"
          fields={{ email: limit.email }}
          variant="outline"
          size="sm"
        >
          Let use everything
        </PostButton>
      </TableCell>
    </TableRow>
  )
}

function ModelList({ models }: { models: string[] }) {
  return models.map((model, i) => (
    <span key={model}>
      {i > 0 && ", "}
      <code className="font-mono text-xs break-all">{model}</code>
    </span>
  ))
}

function LimitPerson({
  available,
  people,
}: {
  available: string[]
  people: string[]
}) {
  return (
    <FormDialog
      trigger={<Button variant="outline" />}
      label="Limit a person"
      title="Limit a person"
      words="They are offered only the models you tick."
      action="/admin/models/access"
      submit="Limit this person"
    >
      <Field id="limit-email" label="Person">
        <Input
          id="limit-email"
          name="email"
          type="email"
          list="limit-people"
          placeholder="ada@example.com"
          required
        />
      </Field>
      <datalist id="limit-people">
        {people.map((email) => (
          <option key={email} value={email} />
        ))}
      </datalist>
      <fieldset className="m-0 grid gap-2 border-0 p-0">
        <legend className="mb-2 text-sm font-medium">May use</legend>
        {available.map((model) => (
          <Label key={model} className="font-normal">
            <Checkbox name="model" value={model} />
            {model}
          </Label>
        ))}
      </fieldset>
    </FormDialog>
  )
}
