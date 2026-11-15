import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { PersonFace } from "@/pages/members/faces"
import type { AgentPage } from "@/pages/members/types"
import { useMembersForm } from "@/pages/members/use-members-form"

const person = "flex h-11 items-center gap-3 px-2"

// AgentAccess is who may work with the agent: its maker, and the people they let. Its maker, or an admin, lets more people or
// stops one.
export function AgentAccess({ agent }: { agent: AgentPage }) {
  const { submit, failed, sending } = useMembersForm()
  const action = `/members/agents/${encodeURIComponent(agent.ID)}/access`
  return (
    <section aria-label="Access" className="grid gap-3">
      <p className="m-0 text-sm text-muted-foreground">
        Only the people listed here can work with {agent.Name}: message it, and
        add it to their rooms.
      </p>
      {failed && (
        <p
          role="alert"
          className="m-0 rounded-md bg-bad-wash px-4 py-3 text-sm text-bad"
        >
          {failed}
        </p>
      )}
      <ul
        aria-label={`Who works with ${agent.Name}`}
        className="m-0 grid list-none gap-px rounded-lg border p-1.5"
        data-testid="access-list"
      >
        <li className={person}>
          <PersonFace initials={agent.OwnerInitials} size="sm" />
          <span className="min-w-0 flex-1 truncate text-sm">
            {agent.OwnerName}
          </span>
          <span className="text-sm text-muted-foreground">Made it</span>
        </li>
        {agent.Access.map((each) => (
          <li key={each.Who} className={person}>
            <PersonFace initials={each.Initials} size="sm" />
            <span className="min-w-0 flex-1 truncate text-sm" title={each.Who}>
              {each.Name}
            </span>
            {agent.Mine ? (
              <form
                method="post"
                action={action}
                onSubmit={submit}
                className="m-0"
              >
                <input type="hidden" name="who" value={each.Who} />
                <input type="hidden" name="action" value="remove" />
                <Button
                  type="submit"
                  variant="link"
                  size="sm"
                  className="text-subtle"
                  aria-label={`Remove ${each.Name}`}
                  disabled={sending}
                >
                  Remove
                </Button>
              </form>
            ) : (
              <span className="text-sm text-muted-foreground">
                Can work with it
              </span>
            )}
          </li>
        ))}
      </ul>
      {agent.Mine && (
        <form
          // A person let in empties the field, since the form is drawn again for the longer list.
          key={agent.Access.length}
          method="post"
          action={action}
          onSubmit={submit}
          className="flex items-end gap-2"
          data-testid="access-form"
        >
          <div className="grid flex-1 gap-2">
            <Label htmlFor="access-who">Let somebody work with it</Label>
            <Input
              id="access-who"
              type="email"
              name="who"
              placeholder="name@example.com"
              required
            />
          </div>
          <Button type="submit" disabled={sending}>
            Add
          </Button>
        </form>
      )}
    </section>
  )
}
