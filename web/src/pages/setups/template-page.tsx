import { LockIcon } from "lucide-react"

import { Disclosure } from "@/components/disclosure"
import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody } from "@/components/page"
import { Section } from "@/components/section"
import { Instructions } from "@/components/skill-set/instructions"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { canStart, choosesAgent } from "@/domain/templates"
import { usePageData } from "@/lib/api"
import { cn } from "@/lib/utils"
import { ChooseAgent } from "@/pages/setups/choose-agent"
import type { TemplateData } from "@/pages/setups/types"
import { WorkMark } from "@/pages/setups/work-mark"
import { withBase } from "@/lib/base"

const crumbs = [{ label: "Presets", href: "/presets" }]

// TemplatePage is one ready-made kind of work: what it is for, what it can do and what to try asking, and the way to start it on
// an agent here, with the instructions it follows and its sample files folded away.
export function TemplatePage() {
  const loaded = usePageData<TemplateData>()
  const title = loaded.state === "ready" ? loaded.data.Template.Title : ""
  return (
    <>
      <PageBar title={title || "Preset"} crumbs={crumbs} />
      <PageBody>
        <Loading loaded={loaded}>
          {(page) => <TemplateParts page={page} />}
        </Loading>
      </PageBody>
    </>
  )
}

function TemplateParts({ page }: { page: TemplateData }) {
  const template = page.Template
  return (
    <>
      {page.Refused && <Notice>{page.Refused}</Notice>}
      <section className="mb-8 flex items-center gap-4 max-md:flex-wrap">
        <WorkMark field={template.Industry} name={template.Title} />
        <div className="min-w-0 flex-[1_1_18rem]">
          <p className="m-0 text-base font-semibold">{template.Title}</p>
          <p className="m-0 text-sm text-muted-foreground">
            {template.Industry}
          </p>
          <p className="m-0 mt-2 max-w-[36rem] text-sm text-muted-foreground">
            {template.Summary}
          </p>
        </div>
        <StartTemplate page={page} />
      </section>
      {template.Restricted && <StaysInside inBuilding={page.InBuilding} />}
      <h2 className="m-0 mb-3 text-sm font-semibold">What it can do</h2>
      <ul className="m-0 mb-8 grid list-none grid-cols-[repeat(auto-fit,minmax(min(14rem,100%),1fr))] gap-4 p-0">
        {template.Skills.map((skill) => (
          <li key={skill.Title}>
            <Card className="h-full">
              <CardContent>
                <strong className="block font-heading text-sm">
                  {skill.Title}
                </strong>
                <p className="m-0 mt-1 text-sm text-muted-foreground">
                  {skill.Blurb}
                </p>
              </CardContent>
            </Card>
          </li>
        ))}
      </ul>
      <h2 className="m-0 mb-3 text-sm font-semibold">Try asking</h2>
      <ul className="m-0 mb-8 flex list-none flex-wrap gap-2 p-0">
        {template.Starters.map((starter) => (
          <li
            key={starter}
            className="rounded-full border bg-card px-4 py-2 text-sm"
          >
            {starter}
          </li>
        ))}
      </ul>
      <Disclosure title="Details" className="mb-8">
        <TemplateDetails page={page} />
      </Disclosure>
    </>
  )
}

// StartTemplate starts the template on the first agent here that can take it, or says it cannot be started.
function StartTemplate({ page }: { page: TemplateData }) {
  const template = page.Template
  if (!canStart(template, page.InBuilding, page.Harnesses.length)) {
    return <Button disabled>Start</Button>
  }
  return (
    <form
      method="post"
      action={withBase(`/templates/${encodeURIComponent(template.Name)}`)}
      className="m-0"
    >
      <input type="hidden" name="harness" value={page.Harnesses[0].Name} />
      <Button type="submit" data-testid="start-template">
        Start {template.Title}
      </Button>
    </form>
  )
}

// StaysInside says that restricted work only goes to models in the deployment's own network, and whether there is one.
function StaysInside({ inBuilding }: { inBuilding: boolean }) {
  return (
    <div
      className={cn(
        "mb-8 flex items-start gap-3 rounded-md px-5 py-4",
        inBuilding ? "bg-info-wash" : "bg-warn-wash"
      )}
    >
      <LockIcon
        className={cn("mt-0.5 size-4", inBuilding ? "text-info" : "text-warn")}
        aria-hidden="true"
      />
      <div>
        <strong className="block">Stays inside your network</strong>
        <p className="m-0 text-sm text-muted-foreground">
          {inBuilding
            ? "This work only goes to models in your own network."
            : "It needs a model in your own network, and this deployment has none. gusto will not send this work anywhere else."}
        </p>
      </div>
    </div>
  )
}

function TemplateDetails({ page }: { page: TemplateData }) {
  const template = page.Template
  return (
    <div className="mt-2 grid grid-cols-[minmax(0,1fr)_minmax(0,20rem)] items-start gap-6 max-lg:grid-cols-1">
      <Section title="Instructions it follows">
        <ul className="m-0 list-none divide-y p-0">
          {template.Skills.map((skill) => (
            <li key={skill.Title} className="py-3 first:pt-0 last:pb-0">
              <span className="font-semibold">{skill.Title}</span>
              <Disclosure title="Read" className="mt-1">
                <Instructions html={skill.Instructions} />
              </Disclosure>
            </li>
          ))}
        </ul>
      </Section>
      <aside
        className="grid min-w-0 gap-6"
        aria-label="About this kind of work"
      >
        <Section title="Sample files">
          <ul className="m-0 grid list-none gap-1 p-0 text-sm">
            {template.Files.map((file) => (
              <li key={file}>
                <code className="break-all">{file}</code>
              </li>
            ))}
          </ul>
          <p className="m-0 mt-3 text-xs text-muted-foreground">
            Invented for demonstration.
          </p>
        </Section>
        {choosesAgent(template, page.InBuilding, page.Harnesses.length) && (
          <Section title="Agent">
            <ChooseAgent name={template.Name} agents={page.Harnesses} />
          </Section>
        )}
      </aside>
    </div>
  )
}
