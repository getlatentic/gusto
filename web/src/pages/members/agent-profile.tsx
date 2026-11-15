import type { ReactNode } from "react"

import { AgentIcon } from "@/components/agent-icon"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { ago } from "@/lib/format"
import { FormSelect } from "@/pages/members/form-select"
import { Fact, Facts, sectionTitle } from "@/pages/members/member-parts"
import type { AgentPage, ModelOption } from "@/pages/members/types"
import { useMembersForm } from "@/pages/members/use-members-form"

const row =
  "grid grid-cols-[9rem_minmax(0,1fr)] items-center gap-3 max-sm:grid-cols-1 max-sm:gap-1.5"

// AgentProfile is who the agent is and how it runs. Its maker, or an admin, changes its name, specialty, model and persona here;
// anybody else reads them.
export function AgentProfile({
  agent,
  offered,
}: {
  agent: AgentPage
  offered: ModelOption[]
}) {
  const { submit, failed, sending } = useMembersForm()
  const editing = agent.Mine
  const parts = (
    <>
      <Group editing={editing} title="Profile">
        {editing && <NameFields agent={agent} />}
        <Facts>
          {!editing && (
            <Fact term="Specialty">{agent.Specialty || "Not set"}</Fact>
          )}
          <Fact term="Made by">{agent.OwnerName}</Fact>
          <Fact term="Made">{ago(agent.Created)}</Fact>
        </Facts>
      </Group>
      <Group editing={editing} title="How it runs">
        <HowItRuns agent={agent} offered={offered} />
      </Group>
      <Group editing={editing} title="Persona">
        <Persona agent={agent} />
      </Group>
    </>
  )
  if (!editing) return <div className="grid gap-8">{parts}</div>
  return (
    <form
      method="post"
      action={`/members/agents/${encodeURIComponent(agent.ID)}`}
      onSubmit={submit}
      className="grid gap-8"
      data-testid="agent-profile-form"
    >
      <input type="hidden" name="tab" value="profile" />
      {failed && (
        <p
          role="alert"
          className="m-0 rounded-md bg-bad-wash px-4 py-3 text-sm text-bad"
        >
          {failed}
        </p>
      )}
      {parts}
      <div>
        <Button type="submit" disabled={sending} data-testid="save-profile">
          Save
        </Button>
      </div>
    </form>
  )
}

// Group is one part of the profile: a fieldset of the form for its maker, and a section to read for anybody else.
function Group({
  editing,
  title,
  children,
}: {
  editing: boolean
  title: string
  children: ReactNode
}) {
  if (editing) {
    return (
      <fieldset className="m-0 grid gap-3 border-0 p-0">
        <legend className={`${sectionTitle} mb-3`}>{title}</legend>
        {children}
      </fieldset>
    )
  }
  return (
    <section aria-label={title} className="grid gap-3">
      <h3 className={sectionTitle}>{title}</h3>
      {children}
    </section>
  )
}

function NameFields({ agent }: { agent: AgentPage }) {
  return (
    <>
      <div className={row}>
        <Label htmlFor="agent-name">Name</Label>
        <Input
          id="agent-name"
          name="name"
          defaultValue={agent.Name}
          maxLength={32}
          required
        />
      </div>
      <div className={row}>
        <Label htmlFor="agent-specialty">Specialty</Label>
        <Input
          id="agent-specialty"
          name="specialty"
          defaultValue={agent.Specialty}
          placeholder="What teammates should ask it for"
          maxLength={80}
        />
      </div>
    </>
  )
}

function HowItRuns({
  agent,
  offered,
}: {
  agent: AgentPage
  offered: ModelOption[]
}) {
  return (
    <>
      <Facts>
        <Fact term="Agent type">
          <span className="flex items-center gap-2">
            {agent.Mark && <AgentIcon src={agent.Mark} className="size-3.5" />}
            {agent.Engine}
          </span>
        </Fact>
        <Fact term="Where">
          In a sandbox on this server. The model's key never reaches it, and
          what it asks of other sites is checked.
        </Fact>
        {!agent.Mine && <Fact term="Model">{agent.Brain || "Default"}</Fact>}
      </Facts>
      {agent.Mine && (
        <div className={row}>
          <Label htmlFor="agent-model">Model</Label>
          <FormSelect
            key={agent.Model}
            id="agent-model"
            name="model"
            value={agent.Model}
            groups={[
              { choices: [{ value: "", label: "Default" }, ...offered] },
            ]}
          />
        </div>
      )}
    </>
  )
}

function Persona({ agent }: { agent: AgentPage }) {
  return (
    <>
      <p id="persona-note" className="m-0 text-sm text-muted-foreground">
        How it works and talks. It is told this at the start of every turn, in
        every room.
      </p>
      {agent.Mine ? (
        <Textarea
          key={agent.Persona}
          name="persona"
          data-testid="persona"
          aria-label="Persona"
          aria-describedby="persona-note"
          className="min-h-40"
          defaultValue={agent.Persona}
          placeholder="You are careful and direct. You say what you checked and what you did not."
        />
      ) : agent.Persona ? (
        <p className="m-0 text-sm whitespace-pre-wrap">{agent.Persona}</p>
      ) : (
        <p className="m-0 text-sm text-subtle">None set.</p>
      )}
    </>
  )
}
