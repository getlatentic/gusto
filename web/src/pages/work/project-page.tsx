import { FileIcon, PlusIcon } from "lucide-react"

import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody } from "@/components/page"
import { Button, buttonVariants } from "@/components/ui/button"
import { dataOf, usePageData } from "@/lib/api"
import { FormFailed } from "@/pages/work/form-failed"
import { ProjectAbout, ProjectRemove } from "@/pages/work/project-about"
import { ProjectApps } from "@/pages/work/project-apps"
import { ProjectConversations } from "@/pages/work/project-conversations"
import { ProjectMark } from "@/pages/work/project-mark"
import { ProjectRoutines } from "@/pages/work/project-routines"
import { ProjectTasks } from "@/pages/work/project-tasks"
import type { ProjectData } from "@/pages/work/types"
import { useWorkForm } from "@/pages/work/use-work-form"

// While an app is starting, the page reads itself again this often, until it answers.
const startingEvery = 3000

// ProjectPage is one agent's computer: what it runs and works in, every conversation held with it, its routines, the apps it
// shares, the checked tasks run on it, and the way to change its model or remove it.
export function ProjectPage() {
  const loaded = usePageData<ProjectData>()
  const refresh = dataOf(loaded)?.Refresh
  const polled = usePageData<ProjectData>({
    pollingInterval: refresh ? startingEvery : 0,
  })
  const page = dataOf(polled)
  return (
    <>
      <PageBar
        title={page?.Agent.Name ?? "Agent"}
        crumbs={[{ label: "Files", href: "/files" }]}
      >
        {page && <ProjectActions id={page.Instance.ID} />}
      </PageBar>
      <PageBody className="max-w-6xl">
        <Loading loaded={polled}>{(data) => <Project page={data} />}</Loading>
      </PageBody>
    </>
  )
}

function ProjectActions({ id }: { id: string }) {
  const form = useWorkForm()
  const at = `/instances/${encodeURIComponent(id)}`
  return (
    <>
      <a
        href={`${at}/files`}
        className={buttonVariants({ variant: "ghost", size: "sm" })}
      >
        <FileIcon aria-hidden="true" /> Files
      </a>
      <form
        method="post"
        action={`${at}/threads`}
        onSubmit={form.submit}
        className="m-0 flex-none"
        data-testid="new-conversation"
      >
        <Button type="submit" size="sm" disabled={form.sending}>
          <PlusIcon aria-hidden="true" /> New conversation
        </Button>
        <FormFailed failed={form.failed} />
      </form>
    </>
  )
}

function Project({ page }: { page: ProjectData }) {
  return (
    <>
      {page.Refused && <Notice>{page.Refused}</Notice>}
      <header className="mb-6 flex items-center gap-4">
        <ProjectMark
          mark={page.Agent.Mark}
          field={page.Agent.Field}
          name={page.Agent.Name}
        />
        <div className="min-w-0 flex-1">
          <p className="m-0 text-base font-semibold wrap-anywhere">
            {page.Agent.Name}
          </p>
          <p className="m-0 text-sm text-subtle">
            {page.Agent.Brain}
            {page.Instance.Sensitivity === "restricted" && (
              <>
                {" · "}
                <span className="font-medium text-warn">
                  stays in your network
                </span>
              </>
            )}
          </p>
        </div>
      </header>
      <div className="grid grid-cols-[minmax(0,1fr)_20rem] items-start gap-6 max-lg:grid-cols-1">
        <div className="grid min-w-0 gap-6">
          <ProjectConversations page={page} />
          <ProjectRoutines page={page} />
          {page.Sharing && <ProjectApps page={page} />}
          <ProjectTasks page={page} />
        </div>
        <aside className="grid min-w-0 gap-4" aria-label="About this agent">
          <ProjectAbout page={page} />
          <ProjectRemove id={page.Instance.ID} />
        </aside>
      </div>
    </>
  )
}
