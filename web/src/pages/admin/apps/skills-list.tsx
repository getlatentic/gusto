import { AppLink } from "@/components/app-link"
import { Card } from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { AddSkill } from "@/pages/admin/apps/add-skill"
import { OnBadge } from "@/pages/admin/apps/on-badge"
import type { AppsData, SkillRow } from "@/pages/admin/apps/types"

// Every skill set written here and every plugin imported, in one list, with the ways to add another above it.
export function SkillsList({ page }: { page: AppsData }) {
  const skills = page.Skills ?? []
  return (
    <>
      <AddSkill
        writes={page.Writes ?? false}
        imports={page.Imports ?? false}
        servers={page.Servers ?? []}
        imported={page.Imported ?? []}
      />
      {skills.length > 0 ? (
        <>
          <Card className="gap-0 py-0">
            <ul className="m-0 list-none divide-y p-0" aria-label="Skills">
              {skills.map((skill) => (
                <SkillLine key={skill.Href} skill={skill} />
              ))}
            </ul>
          </Card>
          <p className="m-0 mt-3 text-sm text-muted-foreground">
            Open one to turn it on or off, or to change what it holds.
          </p>
        </>
      ) : (
        <NoSkills
          writes={page.Writes ?? false}
          imports={page.Imports ?? false}
        />
      )}
    </>
  )
}

function SkillLine({ skill }: { skill: SkillRow }) {
  return (
    <li className="flex items-center gap-4 px-4 py-3">
      <div className="min-w-0 flex-1">
        <AppLink
          href={skill.Href}
          className="block truncate text-sm font-medium text-foreground no-underline underline-offset-2 hover:underline"
        >
          {skill.Name}
        </AppLink>
        <p className="m-0 text-sm text-muted-foreground">
          {skill.Kind} · {skill.Detail}
        </p>
      </div>
      <OnBadge on={skill.On} />
    </li>
  )
}

function NoSkills({ writes, imports }: { writes: boolean; imports: boolean }) {
  let more = ""
  if (writes)
    more = imports
      ? " Write one here, or import an Agent Plugin."
      : " Write one here."
  else if (imports) more = " Import an Agent Plugin to add one."
  return (
    <Empty className="border border-dashed">
      <EmptyHeader>
        <EmptyTitle>No skills yet</EmptyTitle>
        <EmptyDescription>
          A skill set gives agents the instructions and tools for one kind of
          work.{more}
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}
