import { Loading } from "@/components/controls"
import { FormSelect } from "@/components/form-select"
import { Notice, PageBar, PageBody } from "@/components/page"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { chosenSetup } from "@/domain/set-up"
import { usePageData } from "@/lib/api"
import { FormField } from "@/pages/setups/form-field"
import type { SetUpData } from "@/pages/setups/types"
import { WorkMark } from "@/pages/setups/work-mark"

const crumbs = [{ label: "Start an agent", href: "/presets" }]

// SetUpPage is the step between picking an agent type and having an agent: what it thinks with, what it starts out working on,
// and the skill set it is given.
export function SetUpPage() {
  const loaded = usePageData<SetUpData>()
  const title = loaded.state === "ready" ? loaded.data.Harness.Title : ""
  return (
    <>
      <PageBar title={title || "Start an agent"} crumbs={crumbs} />
      <PageBody className="max-w-3xl">
        <Loading loaded={loaded}>
          {(page) => (
            <>
              {page.Refused && <Notice>{page.Refused}</Notice>}
              <div className="mb-6 flex items-center gap-4">
                <WorkMark mark={page.Harness.Mark} name={page.Harness.Title} />
                <div className="min-w-0 flex-1">
                  <p className="m-0 text-base font-semibold">
                    {page.Harness.Title}
                  </p>
                  <p className="m-0 text-sm text-muted-foreground">
                    {page.Harness.What}
                  </p>
                </div>
              </div>
              <SetUpForm page={page} />
            </>
          )}
        </Loading>
      </PageBody>
    </>
  )
}

function SetUpForm({ page }: { page: SetUpData }) {
  return (
    <Card className="max-w-xl">
      <CardContent>
        <form
          className="grid gap-4"
          method="post"
          action="/instances"
          data-testid="set-up-form"
        >
          <input type="hidden" name="harness" value={page.Harness.Name} />
          {page.Then && <input type="hidden" name="then" value={page.Then} />}
          <FormField id="set-up-model" label="Model">
            {page.Models.length > 0 ? (
              <FormSelect
                id="set-up-model"
                name="model"
                value={page.Models[0].value}
                groups={[{ choices: page.Models }]}
              />
            ) : (
              <Input
                id="set-up-model"
                name="model"
                placeholder="the model this deployment reaches"
              />
            )}
          </FormField>
          <FormField
            id="set-up-repo"
            label="Repository"
            hint="optional; without one it starts in an empty folder"
          >
            <Input
              id="set-up-repo"
              name="repo"
              placeholder="git@github.com:you/repo.git"
            />
          </FormField>
          {page.Setups.length > 0 && (
            <FormField
              id="set-up-setup"
              label="Setup"
              hint="skills and MCP servers to give it"
            >
              <FormSelect
                id="set-up-setup"
                name="setup"
                value={chosenSetup(page.Setups, page.Chosen)}
                groups={[
                  { choices: [{ value: "", label: "none" }, ...page.Setups] },
                ]}
              />
            </FormField>
          )}
          <div>
            <Button type="submit" data-testid="start-set-up">
              Start
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
