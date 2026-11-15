import { ChevronRightIcon, InfoIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { Loading } from "@/components/controls"
import { FormSelect } from "@/components/form-select"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { usePageData } from "@/lib/api"
import { FormField } from "@/pages/setups/form-field"
import type { BlankData } from "@/pages/setups/types"

const crumbs = [{ label: "New agent", href: "/presets" }]

// BlankPage starts an agent with nothing set up: its agent type, a skill set if the person has one, and a name.
export function BlankPage() {
  const loaded = usePageData<BlankData>()
  return (
    <>
      <PageBar title="Your own agent" crumbs={crumbs} />
      <PageBody className="max-w-3xl">
        <PageSub>
          Pick its agent type, give it a skill set if you have one, and name it.
        </PageSub>
        <Loading loaded={loaded}>
          {(page) => (
            <>
              {page.Refused && <Notice>{page.Refused}</Notice>}
              {page.Engines.length > 0 ? (
                <BlankForm page={page} />
              ) : (
                <NothingToStart />
              )}
            </>
          )}
        </Loading>
      </PageBody>
    </>
  )
}

function BlankForm({ page }: { page: BlankData }) {
  return (
    <Card className="max-w-xl">
      <CardContent>
        <form
          className="grid gap-4"
          method="post"
          action="/instances"
          data-testid="blank-form"
        >
          <FormField id="blank-title" label="Name it">
            <Input
              id="blank-title"
              name="title"
              placeholder="Agent"
              maxLength={60}
            />
          </FormField>
          <fieldset className="m-0 grid min-w-0 gap-2 border-0 p-0">
            <legend className="mb-2 text-sm font-medium">Agent type</legend>
            <RadioGroup
              name="harness"
              defaultValue={page.Engines[0].Name}
              className="gap-2"
            >
              {page.Engines.map((engine) => (
                <Label
                  key={engine.Name}
                  className="grid cursor-pointer grid-cols-[auto_1fr] items-baseline gap-x-3 rounded-md border px-4 py-3 font-normal hover:border-foreground has-data-checked:border-primary has-data-checked:bg-accent"
                >
                  <RadioGroupItem
                    value={engine.Name}
                    className="row-span-2 self-center"
                  />
                  <span className="font-semibold">{engine.Title}</span>
                  <span className="col-start-2 text-sm text-muted-foreground">
                    {engine.What}
                  </span>
                </Label>
              ))}
            </RadioGroup>
            <AppLink
              href="/engines"
              className="mt-1 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
            >
              How each agent type did{" "}
              <ChevronRightIcon className="size-4" aria-hidden="true" />
            </AppLink>
          </fieldset>
          {page.Setups.length > 0 && (
            <FormField id="blank-setup" label="Skill set">
              <FormSelect
                id="blank-setup"
                name="setup"
                value=""
                groups={[
                  {
                    choices: [
                      { value: "", label: "none" },
                      ...page.Setups.map((name) => ({
                        value: name,
                        label: name,
                      })),
                    ],
                  },
                ]}
              />
            </FormField>
          )}
          <FormField
            id="blank-repo"
            label="Start from a repository"
            hint="optional"
          >
            <Input
              id="blank-repo"
              name="repo"
              placeholder="git@github.com:you/repo.git"
            />
          </FormField>
          <div>
            <Button type="submit" data-testid="start-blank">
              Start
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

function NothingToStart() {
  return (
    <div className="flex max-w-xl items-start gap-3 rounded-md bg-warn-wash px-5 py-4">
      <InfoIcon className="mt-0.5 size-4 text-warn" aria-hidden="true" />
      <div>
        <strong className="block">Nothing can start here yet</strong>
        <p className="m-0 text-sm text-muted-foreground">
          Ask whoever runs this to add an agent.
        </p>
      </div>
    </div>
  )
}
