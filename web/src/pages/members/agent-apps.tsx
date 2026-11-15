import { LinkIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { buttonVariants } from "@/components/ui/button"
import { appChoices } from "@/domain/members"
import { FormSelect } from "@/components/form-select"
import { emptyNote } from "@/pages/members/member-parts"
import type {
  AgentApp,
  AgentApps as Apps,
  AgentPage,
} from "@/pages/members/types"
import { useMembersForm } from "@/pages/members/use-members-form"

const defaultLink = (
  <a className="text-foreground underline" href="/connections">
    default
  </a>
)

// AgentApps is which of the person's own apps the agent may use when they ask it: each one inherits their default, or is allowed
// or kept from it, sent as soon as it is chosen. Each person chooses only for themselves.
export function AgentApps({ agent, apps }: { agent: AgentPage; apps: Apps }) {
  return (
    <section aria-label="Apps" className="grid gap-3">
      <p className="m-0 text-sm text-muted-foreground">
        Which of your apps {agent.Name} may use when you ask it. Inherit follows
        your {defaultLink}.
      </p>
      <AppsList agent={agent} apps={apps} />
    </section>
  )
}

function AppsList({ agent, apps }: { agent: AgentPage; apps: Apps }) {
  if (!apps.Person)
    return <p className={emptyNote}>Sign in as yourself to choose.</p>
  if (!apps.Works) {
    return (
      <p className={emptyNote}>
        Only the people who work with {agent.Name} choose here. When you ask it
        in a room, your {defaultLink} applies.
      </p>
    )
  }
  if (!apps.Apps?.length) {
    return (
      <p className={emptyNote}>
        No apps connected.{" "}
        <a className="text-foreground underline" href="/connections">
          Connect one
        </a>
        .
      </p>
    )
  }
  return (
    <ul
      aria-label={`Apps ${agent.Name} may use`}
      className="m-0 grid list-none gap-px rounded-lg border p-1.5"
      data-testid="apps-list"
    >
      {apps.Apps.map((app) => (
        <li key={app.ID} className="flex h-12 items-center gap-3 px-2">
          <span
            className="grid size-8 flex-none place-items-center rounded-full bg-sunken text-muted-foreground"
            aria-hidden="true"
          >
            <LinkIcon className="size-4" />
          </span>
          <span className="min-w-0 flex-1 text-sm font-medium">{app.Name}</span>
          <AppChoice agent={agent} app={app} />
        </li>
      ))}
    </ul>
  )
}

function AppChoice({ agent, app }: { agent: AgentPage; app: AgentApp }) {
  const { submit } = useMembersForm()
  if (app.SignInAgain) {
    return (
      <>
        <Badge variant="outline" className="text-warn">
          sign in again
        </Badge>
        <a
          className={buttonVariants({ variant: "outline", size: "sm" })}
          href={`/connections/${encodeURIComponent(app.ID)}/connect`}
        >
          Sign in
        </a>
      </>
    )
  }
  return (
    <form
      method="post"
      action={`/members/agents/${encodeURIComponent(agent.ID)}/apps`}
      onSubmit={submit}
      className="m-0"
    >
      <input type="hidden" name="app" value={app.ID} />
      <FormSelect
        key={app.Choice}
        name="choice"
        label={`What ${agent.Name} may do with your ${app.Name}`}
        value={app.Choice}
        groups={[{ choices: appChoices(app.Default) }]}
        sendOnChoose
        className="h-8 w-44"
      />
    </form>
  )
}
