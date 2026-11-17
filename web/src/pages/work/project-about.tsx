import type { ReactNode } from "react"

import { AppLink } from "@/components/app-link"
import { Button } from "@/components/ui/button"
import { FormSelect } from "@/components/form-select"
import { ConfirmForm } from "@/pages/work/confirm-form"
import { FormFailed } from "@/pages/work/form-failed"
import { SectionCard } from "@/pages/work/section-card"
import type { ProjectData } from "@/pages/work/types"
import { useWorkForm } from "@/pages/work/use-work-form"
import { withBase } from "@/lib/base"

function Item({ term, children }: { term: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-muted-foreground">{term}</dt>
      <dd className="m-0 min-w-0 break-words">{children}</dd>
    </>
  )
}

// ProjectAbout is what the agent is: its model, which it can be pointed at another of from the next message on, its type, its
// skills, its repository and when it started.
export function ProjectAbout({ page }: { page: ProjectData }) {
  const form = useWorkForm()
  const at = `/instances/${encodeURIComponent(page.Instance.ID)}`
  return (
    <SectionCard>
      <dl className="m-0 grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-4 gap-y-3 text-sm">
        <Item term="Model">
          {page.Models.length > 1 ? (
            <form
              method="post"
              action={withBase(`${at}/model`)}
              onSubmit={form.submit}
              className="m-0 flex items-center gap-2"
              data-testid="change-model"
            >
              <FormSelect
                key={page.Instance.Model}
                name="model"
                label="Model"
                value={page.Instance.Model}
                className="min-w-0 flex-1"
                testId="model"
                groups={[{ choices: page.Models }]}
              />
              <Button
                type="submit"
                variant="outline"
                size="sm"
                disabled={form.sending}
              >
                Use
              </Button>
            </form>
          ) : (
            page.Agent.Brain
          )}
        </Item>
        <Item term="Agent type">{page.Agent.Engine}</Item>
        {page.Instance.Setup && (
          <Item term="Skills">
            <AppLink
              className="font-medium text-foreground"
              href={`/setups/${encodeURIComponent(page.Instance.Setup)}`}
            >
              {page.Instance.Setup}
            </AppLink>
          </Item>
        )}
        {page.Instance.Repo && (
          <Item term="Repository">{page.Instance.Repo}</Item>
        )}
        <Item term="Started">{page.Instance.Started}</Item>
      </dl>
      <FormFailed failed={form.failed} />
    </SectionCard>
  )
}

// ProjectRemove removes the agent, which is asked first. Its files are kept aside.
export function ProjectRemove({ id }: { id: string }) {
  const form = useWorkForm()
  return (
    <SectionCard>
      <ConfirmForm
        action={`/instances/${encodeURIComponent(id)}/remove`}
        question="Remove this agent? Its files are kept aside and its activity stays in the log."
        yes="Remove"
        send={form.send}
        testId="remove-agent"
      >
        <Button type="submit" variant="destructive">
          Remove this agent
        </Button>
      </ConfirmForm>
      <p className="m-0 text-xs text-muted-foreground">
        Its files are kept aside, not deleted.
      </p>
      <FormFailed failed={form.failed} />
    </SectionCard>
  )
}
