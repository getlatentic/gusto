import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Disclosure } from "@/pages/admin/apps/disclosure"
import { Field } from "@/pages/admin/apps/field"
import { Section } from "@/pages/admin/apps/section"
import type {
  EarlierSkill,
  SkillOfSet,
  SkillSetData,
} from "@/pages/admin/apps/types"

// SetSkills is each skill of the set: when an agent uses it, its instructions folded away, and the way to remove it.
export function SetSkills({ set }: { set: SkillSetData["Set"] }) {
  return (
    <Section title="Skills" count={set.Skills.length}>
      {set.Skills.length > 0 ? (
        <ul className="m-0 list-none divide-y p-0">
          {set.Skills.map((skill) => (
            <SkillLine key={skill.Name} name={set.Name} skill={skill} />
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

function SkillLine({ name, skill }: { name: string; skill: SkillOfSet }) {
  return (
    <li className="py-4 first:pt-0 last:pb-0">
      <div className="flex items-center justify-between gap-3">
        <span className="min-w-0 font-semibold break-words">{skill.Name}</span>
        <form
          method="post"
          action={`/admin/skill-sets/${name}/skills/${skill.Name}/drop`}
          className="m-0"
        >
          <Button variant="destructive" size="sm" type="submit">
            Remove
          </Button>
        </form>
      </div>
      <p className="m-0 mt-1 mb-2 text-sm text-muted-foreground">
        {skill.Description}
      </p>
      <Disclosure title="Instructions">
        <Instructions html={skill.Instructions} />
      </Disclosure>
    </li>
  )
}

// Instructions are a skill's body as the server drew it from Markdown: it draws HTML that was written as text, so none of it runs.
function Instructions({ html }: { html: string }) {
  return (
    <div
      className="mt-1 max-w-[72ch] text-sm break-words [&_a]:text-primary [&_a]:underline [&_code]:rounded-xs [&_code]:bg-muted [&_code]:px-1 [&_code]:font-mono [&_h1,&_h2,&_h3,&_h4]:mt-4 [&_h1,&_h2,&_h3,&_h4]:mb-2 [&_h1,&_h2,&_h3,&_h4]:font-semibold [&_li]:my-1 [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:my-2 [&_pre]:overflow-x-auto [&_pre]:rounded-sm [&_pre]:bg-muted [&_pre]:p-3 [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_table]:block [&_table]:max-w-full [&_table]:overflow-x-auto [&_ul]:list-disc [&_ul]:pl-5"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}

// EarlierSkills are the kept versions of the set's skills, each with the way to put it back.
export function EarlierSkills({
  name,
  earlier,
}: {
  name: string
  earlier: EarlierSkill[]
}) {
  return (
    <Section title="Earlier versions">
      <p className="m-0 mb-3 text-sm text-muted-foreground">
        A skill that is changed or removed is kept as it was, ten times over.
        Putting one back is a change like any other.
      </p>
      <ul className="m-0 list-none divide-y p-0 text-sm">
        {earlier.map((kept) => (
          <li
            key={kept.Skill + kept.At}
            className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 first:pt-0 last:pb-0"
          >
            <code className="font-mono">{kept.Skill}</code>
            {!kept.Present && (
              <span className="text-muted-foreground">removed</span>
            )}
            <span className="font-mono text-muted-foreground">
              {kept.Fingerprint}
            </span>
            <form
              method="post"
              action={`/admin/skill-sets/${name}/skills/${kept.Skill}/restore`}
              className="m-0 ml-auto"
            >
              <input type="hidden" name="at" value={kept.At} />
              <Button variant="outline" size="sm" type="submit">
                Put it back
              </Button>
            </form>
          </li>
        ))}
      </ul>
    </Section>
  )
}

// AddSkillToSet writes a new skill into the set, or writes over the one of that name.
export function AddSkillToSet({ name }: { name: string }) {
  return (
    <Section title="Add a skill">
      <form
        className="grid gap-4"
        method="post"
        action={`/admin/skill-sets/${name}/skills`}
      >
        <Field id="skill-name" label="Name">
          <Input
            id="skill-name"
            name="skill"
            placeholder="review-a-diff"
            pattern="[a-z0-9][a-z0-9\-]*"
            required
          />
        </Field>
        <Field
          id="skill-description"
          label="When to use it"
          hint="the agent decides by this"
        >
          <Input
            id="skill-description"
            name="description"
            placeholder="Use when asked to review a change before it is merged."
            required
          />
        </Field>
        <Field id="skill-body" label="Instructions">
          <Textarea
            id="skill-body"
            name="body"
            rows={8}
            placeholder="Read the whole diff before commenting. Name the file and line of each problem."
            required
          />
        </Field>
        <div>
          <Button type="submit">Add the skill</Button>
        </div>
      </form>
    </Section>
  )
}
