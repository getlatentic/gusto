import type { ReactNode } from "react"

import { Disclosure } from "@/components/disclosure"
import { Section } from "@/components/section"
import { Instructions } from "@/components/skill-set/instructions"
import type { SkillOfSet } from "@/components/skill-set/types"

// SetSkills is each skill of a set: when an agent uses it, and its instructions folded away. The console gives each skill a
// control to change it; a person's page gives none.
export function SetSkills({
  skills,
  control,
}: {
  skills: SkillOfSet[]
  control?: (skill: SkillOfSet) => ReactNode
}) {
  return (
    <Section title="Skills" count={skills.length}>
      {skills.length > 0 ? (
        <ul className="m-0 list-none divide-y p-0">
          {skills.map((skill) => (
            <li key={skill.Name} className="py-4 first:pt-0 last:pb-0">
              <div className="flex items-center justify-between gap-3">
                <span className="min-w-0 font-semibold break-words">
                  {skill.Name}
                </span>
                {control?.(skill)}
              </div>
              <p className="m-0 mt-1 mb-2 text-sm text-muted-foreground">
                {skill.Description}
              </p>
              <Disclosure title="Instructions">
                <Instructions html={skill.Instructions} />
              </Disclosure>
            </li>
          ))}
        </ul>
      ) : (
        <p className="m-0 text-sm text-muted-foreground">
          No skills yet. A skill is instructions an agent reads when its
          description matches what it was asked.
        </p>
      )}
    </Section>
  )
}
