import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react"
import { Navigate } from "react-router"

import { AppLink } from "@/components/app-link"
import { Loading } from "@/components/controls"
import { PageBar, PageBody } from "@/components/page"
import { Button, buttonVariants } from "@/components/ui/button"
import { dataOf, usePageData, useRefreshPage } from "@/lib/api"
import { CutNote } from "@/pages/work/cut-note"
import { TaskChecks, TaskNetwork, TaskSteps } from "@/pages/work/task-checks"
import { TaskDetails, TaskSummary } from "@/pages/work/task-details"
import { TaskPatch } from "@/pages/work/task-patch"
import { TaskNotes } from "@/pages/work/task-notes"
import { TaskProof } from "@/pages/work/task-proof"
import { SectionCard } from "@/pages/work/section-card"
import type { TaskData } from "@/pages/work/types"
import { useWatch, type Watched } from "@/pages/work/use-watch"
import { useWorkForm } from "@/pages/work/use-work-form"
import { FormFailed } from "@/pages/work/form-failed"
import { withBase } from "@/lib/base"

// TaskPage is one run and everything it left: what it said, what it did and changed, whether its check passed, what its calls
// to a model cost, and the record that vouches for all of it. While it runs, its words arrive as it says them. A turn of a
// conversation is read in its conversation by anybody but an admin, and the page goes there.
export function TaskPage() {
  const loaded = usePageData<TaskData>()
  const page = dataOf(loaded)
  if (page?.MovedTo) return <Navigate to={page.MovedTo} replace />
  if (!page) {
    return (
      <>
        <PageBar title="Run details" />
        <PageBody className="max-w-5xl">
          <Loading loaded={loaded}>{() => null}</Loading>
        </PageBody>
      </>
    )
  }
  return <TaskRun key={page.Job.ID} page={page} />
}

function TaskRun({ page }: { page: TaskData }) {
  const refresh = useRefreshPage()
  const watched = useWatch(page.Job.ID, Boolean(page.Live), refresh)
  return (
    <>
      <TaskBar page={page} />
      <PageBody className="max-w-5xl">
        <div className="mb-5">
          <p
            className="m-0 text-lg font-semibold wrap-anywhere"
            data-testid="task-brief"
          >
            {page.Job.Brief}
          </p>
          {page.Talk && (
            <p className="m-0 mt-1 text-sm text-subtle">
              Message {page.Talk.Place} of {page.Talk.Of}
            </p>
          )}
          {page.Waiting && (
            <p className="m-0 mt-1 text-sm text-muted-foreground" id="why">
              {page.Waiting}
            </p>
          )}
        </div>
        <TaskSummary page={page} ended={watched.ended} />
        <TaskDetails page={page} />
        <TaskNotes page={page} />
        <div className="grid max-w-4xl gap-4">
          <TaskWords page={page} watched={watched} />
          <TaskPatch page={page} />
          <TaskSteps steps={page.Steps ?? []} />
          <TaskNetwork page={page} />
          <TaskChecks page={page} />
          {page.Job.Repo && (
            <p className="m-0 text-xs text-muted-foreground">
              Repository: <span className="wrap-anywhere">{page.Job.Repo}</span>
            </p>
          )}
          <TaskProof page={page} />
        </div>
      </PageBody>
    </>
  )
}

// TaskBar is where the run is, the messages either side of a turn, the way to its conversation, and, while it runs, Stop.
function TaskBar({ page }: { page: TaskData }) {
  const crumbs = page.Agent
    ? [
        { label: "Files", href: "/files" },
        {
          label: page.Agent.Name,
          href: `/instances/${encodeURIComponent(page.Agent.ID)}`,
        },
      ]
    : [{ label: "Activity", href: "/jobs" }]
  if (page.Talk && page.ThreadPath)
    crumbs.push({ label: page.Talk.Name, href: page.ThreadPath })
  const step = buttonVariants({ variant: "outline", size: "sm" })
  return (
    <PageBar title="Run details" crumbs={crumbs}>
      <div className="flex flex-none items-center gap-2">
        {page.Talk?.Before && (
          <AppLink
            href={`/jobs/${encodeURIComponent(page.Talk.Before)}`}
            className={step}
          >
            <ChevronLeftIcon aria-hidden="true" /> Previous
          </AppLink>
        )}
        {page.Talk?.After && (
          <AppLink
            href={`/jobs/${encodeURIComponent(page.Talk.After)}`}
            className={step}
          >
            Next <ChevronRightIcon aria-hidden="true" />
          </AppLink>
        )}
        {page.ThreadPath && (
          <AppLink
            href={page.ThreadPath}
            className={buttonVariants({ size: "sm" })}
          >
            Open the conversation
          </AppLink>
        )}
        {page.Live && <StopTask id={page.Job.ID} />}
      </div>
    </PageBar>
  )
}

function StopTask({ id }: { id: string }) {
  const form = useWorkForm()
  const at = `/jobs/${encodeURIComponent(id)}`
  return (
    <form
      method="post"
      action={withBase(`${at}/stop`)}
      onSubmit={form.submit}
      className="m-0"
      data-testid="stop-task"
    >
      <input type="hidden" name="back" value={at} />
      <Button
        type="submit"
        variant="destructive"
        size="sm"
        disabled={form.sending}
      >
        Stop
      </Button>
      <FormFailed failed={form.failed} />
    </form>
  )
}

// TaskWords is what the run said: as it says it while it runs, and its transcript once it is over. A turn's words are read in
// its conversation.
function TaskWords({ page, watched }: { page: TaskData; watched: Watched }) {
  if (page.Live) {
    return (
      <SectionCard title="Running now">
        <pre
          id="stream"
          data-testid="task-stream"
          className="m-0 max-h-[28rem] overflow-auto rounded-sm bg-muted p-4 whitespace-pre-wrap text-muted-foreground"
        >
          {watched.said || "waiting…"}
        </pre>
      </SectionCard>
    )
  }
  const transcript = page.Transcript
  if (page.Talk || !transcript) return null
  return (
    <SectionCard title="What it said" testId="task-transcript">
      {transcript.Empty ? (
        <p className="m-0 text-muted-foreground">It said nothing.</p>
      ) : (
        <>
          <div
            className="prose min-w-0"
            // The transcript's markdown is drawn and made safe by the server, which escapes what the harness wrote.
            dangerouslySetInnerHTML={{ __html: transcript.HTML }}
          />
          <CutNote cut={transcript} />
        </>
      )}
    </SectionCard>
  )
}
