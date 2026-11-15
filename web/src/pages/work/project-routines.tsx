import { AppLink } from "@/components/app-link"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { routineLast } from "@/domain/work"
import { ConfirmForm } from "@/pages/work/confirm-form"
import { Disclosure } from "@/pages/work/disclosure"
import { Field } from "@/pages/work/field"
import { FormFailed } from "@/pages/work/form-failed"
import { SectionCard } from "@/pages/work/section-card"
import { StateBadge } from "@/pages/work/state-badge"
import type { ProjectData, Routine } from "@/pages/work/types"
import { useWorkForm } from "@/pages/work/use-work-form"

const lastStates = { ok: "passed", warn: "queued", plain: "", bad: "failed" }

// ProjectRoutines is what the agent is asked on a schedule, each in its own conversation, and the way to add one.
export function ProjectRoutines({ page }: { page: ProjectData }) {
  const form = useWorkForm()
  const at = `/instances/${encodeURIComponent(page.Instance.ID)}`
  return (
    <SectionCard title="Routines" id="routines" testId="routines">
      {page.Unread && (
        <p
          role="alert"
          className="m-0 rounded-md bg-bad-wash px-4 py-3 text-sm text-bad"
        >
          No routine runs until this is mended: {page.Unread}
        </p>
      )}
      {page.Routines.length > 0 && (
        <ul className="m-0 list-none divide-y border-y p-0">
          {page.Routines.map((routine) => (
            <RoutineLine
              key={routine.ID}
              routine={routine}
              at={at}
              send={form.send}
            />
          ))}
        </ul>
      )}
      <FormFailed failed={form.failed} />
      <Disclosure summary="Add a routine" testId="add-routine">
        <form
          method="post"
          action={`${at}/routines`}
          onSubmit={form.submit}
          className="grid gap-4"
        >
          <div className="grid grid-cols-2 gap-4 max-md:grid-cols-1">
            <div className="grid content-start gap-4">
              <Field label="Title">
                <Input name="title" placeholder="Morning summary" required />
              </Field>
              <Field label="Daily at">
                <Input name="daily" placeholder="08:00" />
              </Field>
              <Field label="or every" hint="hours">
                <Input name="every" inputMode="numeric" placeholder="6" />
              </Field>
            </div>
            <Field label="What to ask">
              <Textarea
                name="prompt"
                className="h-full"
                placeholder="Summarize what changed in the repository since yesterday."
                required
              />
            </Field>
          </div>
          <div>
            <Button type="submit" disabled={form.sending}>
              Add it
            </Button>
          </div>
        </form>
      </Disclosure>
    </SectionCard>
  )
}

function RoutineLine({
  routine,
  at,
  send,
}: {
  routine: Routine
  at: string
  send: (form: HTMLFormElement) => Promise<void>
}) {
  const last = routineLast(routine.Last)
  const mine = `${at}/routines/${encodeURIComponent(routine.ID)}`
  const turn = routine.Paused ? "resume" : "pause"
  return (
    <li
      className="flex items-center gap-3 py-3 max-md:flex-wrap"
      data-testid="routine"
    >
      <div className="min-w-0 flex-1">
        <AppLink
          className="block truncate font-medium text-foreground no-underline hover:underline"
          href={`${at}/threads/${encodeURIComponent(routine.Thread)}`}
          title={routine.Prompt}
        >
          {routine.Title}
        </AppLink>
        <p className="m-0 mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <span>{routine.When}</span>
          {routine.Paused && <StateBadge state="queued">Paused</StateBadge>}
          <StateBadge state={lastStates[last.tone]}>{last.words}</StateBadge>
        </p>
      </div>
      <div className="flex flex-none items-center gap-2">
        <form
          method="post"
          action={`${mine}/${turn}`}
          className="m-0"
          onSubmit={(event) => {
            event.preventDefault()
            void send(event.currentTarget)
          }}
        >
          <Button type="submit" variant="outline" size="sm">
            {routine.Paused ? "Resume" : "Pause"}
          </Button>
        </form>
        <ConfirmForm
          action={`${mine}/remove`}
          question="Remove this routine? Its conversation is kept."
          yes="Remove"
          send={send}
        >
          <Button type="submit" variant="destructive" size="sm">
            Remove
          </Button>
        </ConfirmForm>
      </div>
    </li>
  )
}
