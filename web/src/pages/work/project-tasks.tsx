import { AppLink } from "@/components/app-link"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { ago } from "@/lib/format"
import { FormSelect } from "@/pages/members/form-select"
import { Disclosure } from "@/pages/work/disclosure"
import { Field } from "@/pages/work/field"
import { FormFailed } from "@/pages/work/form-failed"
import { SectionCard } from "@/pages/work/section-card"
import { StateBadge } from "@/pages/work/state-badge"
import type { ProjectData } from "@/pages/work/types"
import { useWorkForm } from "@/pages/work/use-work-form"

const sensitivities = [
  { value: "internal", label: "internal" },
  { value: "public", label: "public" },
  { value: "restricted", label: "restricted: the model must be local" },
]

// ProjectTasks is the work done on the agent's folder that a check decides, not said in a conversation, and the way to run
// another.
export function ProjectTasks({ page }: { page: ProjectData }) {
  const form = useWorkForm()
  return (
    <SectionCard id="task" testId="tasks">
      <Disclosure open={page.Tasks.length > 0} heading summary="Checked tasks">
        {page.Tasks.length > 0 && (
          <ol className="m-0 mb-4 list-none divide-y p-0">
            {page.Tasks.map((task) => (
              <li key={task.ID}>
                <AppLink
                  href={`/jobs/${encodeURIComponent(task.ID)}`}
                  className="grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-4 py-3 text-foreground no-underline hover:bg-muted/50"
                >
                  <span className="truncate font-medium" title={task.Brief}>
                    {task.Brief}
                  </span>
                  <StateBadge state={task.State}>{task.State}</StateBadge>
                  <span className="text-xs text-subtle">
                    {ago(task.Queued)}
                  </span>
                </AppLink>
              </li>
            ))}
          </ol>
        )}
        <form
          method="post"
          action={`/instances/${encodeURIComponent(page.Instance.ID)}/tasks`}
          onSubmit={form.submit}
          className="grid gap-4"
        >
          <Field label="What to do">
            <Textarea
              name="brief"
              placeholder="Write a script that prints the ten largest files here."
              required
            />
          </Field>
          <Field label="Check" hint="the command that says it worked">
            <Input name="gate" placeholder="sh test.sh" required />
          </Field>
          <div className="grid grid-cols-3 gap-4 max-md:grid-cols-1">
            <Field label="Sensitivity">
              <FormSelect
                name="sensitivity"
                label="Sensitivity"
                value="internal"
                groups={[{ choices: sensitivities }]}
              />
            </Field>
            <Field label="Team" hint="optional">
              <Input name="team" placeholder="platform" />
            </Field>
            <Field label="Cap in dollars" hint="optional">
              <Input name="usd" inputMode="decimal" placeholder="5.00" />
            </Field>
          </div>
          <div>
            <Button type="submit" disabled={form.sending}>
              Run it
            </Button>
          </div>
        </form>
        <FormFailed failed={form.failed} />
      </Disclosure>
    </SectionCard>
  )
}
