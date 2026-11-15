import { InfoIcon } from "lucide-react"

import { AgentIcon } from "@/components/agent-icon"
import { AppLink } from "@/components/app-link"
import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { usePageData } from "@/lib/api"
import { ago } from "@/lib/format"
import { FormSelect } from "@/pages/members/form-select"
import { Field } from "@/pages/work/field"
import { FormFailed } from "@/pages/work/form-failed"
import type { CompareData } from "@/pages/work/types"
import { useWorkForm } from "@/pages/work/use-work-form"

// ComparePage gives several agents the same task on the same code, and the project's own tests decide which of them did it.
// The person's earlier comparisons are listed under it.
export function ComparePage() {
  const loaded = usePageData<CompareData>()
  return (
    <>
      <PageBar title="Compare agents" />
      <PageBody>
        <Loading loaded={loaded}>{(page) => <Compare page={page} />}</Loading>
      </PageBody>
    </>
  )
}

function Compare({ page }: { page: CompareData }) {
  return (
    <>
      {page.Refused && <Notice>{page.Refused}</Notice>}
      <PageSub>
        Give several agents the same task on the same code. The project&apos;s
        own tests decide which of them did it.
      </PageSub>
      {page.Template ? (
        <StartComparison page={page} template={page.Template} />
      ) : (
        <div className="flex max-w-xl items-start gap-3 rounded-md bg-warn-wash px-5 py-4">
          <InfoIcon aria-hidden="true" className="size-5 flex-none text-warn" />
          <div>
            <strong className="block">Nothing to compare on yet</strong>
            <p className="m-0 text-sm text-muted-foreground">
              No template here has a check that can decide between agents.
            </p>
          </div>
        </div>
      )}
      {page.Lineups.length > 0 && <Earlier lineups={page.Lineups} />}
    </>
  )
}

function StartComparison({
  page,
  template,
}: {
  page: CompareData
  template: NonNullable<CompareData["Template"]>
}) {
  const form = useWorkForm()
  const models = page.Models ?? []
  return (
    <Card className="max-w-xl py-4">
      <CardContent className="px-4">
        <form
          method="post"
          action="/compare"
          onSubmit={form.submit}
          className="grid gap-4"
          data-testid="start-comparison"
        >
          <Field label="The task">
            <Textarea
              name="brief"
              rows={2}
              defaultValue={template.Starter}
              required
            />
          </Field>
          <p className="m-0 text-xs text-muted-foreground">
            On the {template.Title} sample project, checked by running{" "}
            <code>{template.Check}</code> after each agent.
          </p>
          <fieldset className="m-0 grid gap-2 border-0 p-0">
            <legend className="mb-2 text-sm font-medium">Agents</legend>
            {(page.Engines ?? []).map((engine) => (
              <label
                key={engine.Name}
                data-engine
                className="flex cursor-pointer items-center gap-3 rounded-md border px-4 py-3 hover:border-foreground has-data-checked:border-brand has-data-checked:bg-brand/10"
              >
                <Checkbox name="harness" value={engine.Name} defaultChecked />
                {engine.Mark && <AgentIcon src={engine.Mark} />}
                {engine.Title}
              </label>
            ))}
          </fieldset>
          {models.length > 0 && (
            <Field label="Model">
              <FormSelect
                name="model"
                label="Model"
                value={models[0]}
                groups={[
                  {
                    choices: models.map((model) => ({
                      value: model,
                      label: model,
                    })),
                  },
                ]}
              />
            </Field>
          )}
          <div>
            <Button type="submit" disabled={form.sending}>
              Compare
            </Button>
          </div>
          <FormFailed failed={form.failed} />
        </form>
      </CardContent>
    </Card>
  )
}

function Earlier({ lineups }: { lineups: CompareData["Lineups"] }) {
  return (
    <section className="mt-10" aria-labelledby="earlier">
      <h2 id="earlier" className="m-0 mb-3 text-base font-semibold">
        Earlier comparisons
      </h2>
      <Card className="py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Task</TableHead>
              <TableHead>Model</TableHead>
              <TableHead>Agents</TableHead>
              <TableHead>When</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {lineups.map((lineup) => (
              <TableRow key={lineup.ID}>
                <TableCell>
                  <AppLink
                    className="font-medium"
                    href={`/compare/${encodeURIComponent(lineup.ID)}`}
                  >
                    {lineup.Brief}
                  </AppLink>
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {lineup.Model}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {lineup.Agents}
                </TableCell>
                <TableCell
                  className="text-muted-foreground"
                  title={lineup.When}
                >
                  {ago(lineup.At)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </section>
  )
}
