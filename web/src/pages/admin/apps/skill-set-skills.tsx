import { Section } from "@/components/section"
import { SetSkills as SharedSkills } from "@/components/skill-set/set-skills"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Field } from "@/pages/admin/apps/field"
import type { EarlierSkill, SkillSetData } from "@/pages/admin/apps/types"

// SetSkills is each skill of the set, with the way to remove it.
export function SetSkills({ set }: { set: SkillSetData["Set"] }) {
  return (
    <SharedSkills
      skills={set.Skills}
      control={(skill) => (
        <form
          method="post"
          action={`/admin/skill-sets/${set.Name}/skills/${skill.Name}/drop`}
          className="m-0"
        >
          <Button variant="destructive" size="sm" type="submit">
            Remove
          </Button>
        </form>
      )}
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
