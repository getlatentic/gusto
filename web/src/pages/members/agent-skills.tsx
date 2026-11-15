import { Label } from "@/components/ui/label"
import { skillGroups } from "@/domain/members"
import { FormSelect, type ChoiceGroup } from "@/components/form-select"
import { emptyNote } from "@/pages/members/member-parts"
import type { AgentPage, AgentSkills as Skills } from "@/pages/members/types"
import { useMembersForm } from "@/pages/members/use-members-form"

// AgentSkills is the skills the agent is given in every turn. Its maker, or an admin, gives it a ready-made role or a skill set,
// sent as soon as it is chosen.
export function AgentSkills({
  agent,
  skills,
}: {
  agent: AgentPage
  skills: Skills
}) {
  const { submit, failed } = useMembersForm()
  return (
    <section aria-label="Skills" className="grid gap-3">
      <p className="m-0 text-sm text-muted-foreground">
        The skills {agent.Name} is given in every turn: one of the ready-made
        roles, or a skill set made for this workspace.
      </p>
      {failed && (
        <p
          role="alert"
          className="m-0 rounded-md bg-bad-wash px-4 py-3 text-sm text-bad"
        >
          {failed}
        </p>
      )}
      {agent.Mine && (
        <form
          method="post"
          action={`/members/agents/${encodeURIComponent(agent.ID)}`}
          onSubmit={submit}
          className="grid gap-2"
          data-testid="skills-form"
        >
          <input type="hidden" name="tab" value="skills" />
          <Label htmlFor="agent-skills">Skills</Label>
          <FormSelect
            key={agent.Setup}
            id="agent-skills"
            name="setup"
            value={agent.Setup}
            groups={choicesOf(skills)}
            sendOnChoose
          />
        </form>
      )}
      {skills.Given ? (
        <ul
          aria-label={skills.Title}
          className="m-0 grid list-none gap-px rounded-lg border p-1.5"
        >
          {skills.Skills.length === 0 && (
            <li className="px-3 py-2.5 text-sm text-subtle">
              {skills.Title} holds no skills yet.
            </li>
          )}
          {skills.Skills.map((skill) => (
            <li key={skill.Name} className="grid gap-0.5 px-3 py-2.5">
              <span className="text-sm font-medium">{skill.Name}</span>
              {skill.Description && (
                <span className="text-sm text-muted-foreground">
                  {skill.Description}
                </span>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <p className={emptyNote}>
          No skills given. {agent.Name} works with what its agent type knows.
        </p>
      )}
    </section>
  )
}

function choicesOf(skills: Skills): ChoiceGroup[] {
  const { roles, sets } = skillGroups(skills.Choices ?? [])
  const asChoices = (each: typeof roles) =>
    each.map((choice) => ({ value: choice.Name, label: choice.Title }))
  return [
    { choices: [{ value: "", label: "None" }] },
    { heading: "Ready-made roles", choices: asChoices(roles) },
    { heading: "Skill sets", choices: asChoices(sets) },
  ].filter((group) => group.choices.length > 0)
}
