import { useParams } from "react-router"

import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody } from "@/components/page"
import { Button } from "@/components/ui/button"
import { usePageData } from "@/lib/api"
import { OnBadge } from "@/pages/admin/apps/on-badge"
import { SkillSetSide } from "@/pages/admin/apps/skill-set-side"
import {
  AddSkillToSet,
  EarlierSkills,
  SetSkills,
} from "@/pages/admin/apps/skill-set-skills"
import type { SkillSetData } from "@/pages/admin/apps/types"
import { withBase } from "@/lib/base"

const crumbs = [{ label: "Apps & skills", href: "/admin/apps?tab=skills" }]

// One skill set as an admin changes it: its skills and their earlier versions, its servers and plugins, and whether it is on.
export function SkillSetPage() {
  const name = useParams().name ?? ""
  const loaded = usePageData<SkillSetData>()
  const set = loaded.state === "ready" ? loaded.data.Set : undefined
  return (
    <>
      <PageBar
        crumbs={crumbs}
        title={
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate" title={name}>
              {name}
            </span>
            {set?.Off && <OnBadge on={false} />}
          </span>
        }
      >
        {set && <TurnSet name={set.Name} off={set.Off} />}
      </PageBar>
      <PageBody>
        <Loading loaded={loaded}>
          {(page) => <SkillSetParts page={page} />}
        </Loading>
      </PageBody>
    </>
  )
}

// TurnSet turns the skill set on, which offers it to people, or off.
function TurnSet({ name, off }: { name: string; off: boolean }) {
  return (
    <form
      method="post"
      action={withBase(
        `/admin/skill-sets/${name}/${off ? "enable" : "disable"}`
      )}
      className="m-0 flex-none"
    >
      <Button type="submit" variant={off ? "default" : "outline"}>
        {off ? "Turn on" : "Turn off"}
      </Button>
    </form>
  )
}

function SkillSetParts({ page }: { page: SkillSetData }) {
  const set = page.Set
  return (
    <>
      {page.Refused && <Notice>{page.Refused}</Notice>}
      {set.Off && (
        <Notice tone="warn">
          This skill set is off: it is not offered to anybody, and no agent
          starts with it, until you turn it on.
        </Notice>
      )}
      <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,20rem)] items-start gap-6 max-lg:grid-cols-1">
        <div className="grid min-w-0 gap-6">
          <SetSkills set={set} />
          {page.Earlier.length > 0 && (
            <EarlierSkills name={set.Name} earlier={page.Earlier} />
          )}
          {set.Skills.length > 0 && (
            <p className="m-0 text-sm text-muted-foreground">
              <a
                className="font-medium text-foreground"
                href={withBase(`/admin/skill-sets/${set.Name}/plugin.zip`)}
              >
                Download as an Agent Plugin
              </a>
              : a plugin of these skills and no servers, which another
              deployment, or this one, imports as an integration.
            </p>
          )}
          <AddSkillToSet name={set.Name} />
        </div>
        <SkillSetSide page={page} />
      </div>
    </>
  )
}
