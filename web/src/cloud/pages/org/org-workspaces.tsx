import { PersonForm, Section } from "@/cloud/pages/org/org-parts"
import type { CloudPage, CloudSpace } from "@/cloud/types"
import { FormSelect } from "@/components/form-select"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

// OrgWorkspaces is the organization's workspaces and who is in each, which an admin renames, fills and empties.
export function OrgWorkspaces({ page }: { page: CloudPage }) {
  return (
    <Section title="Workspaces" id="workspaces">
      <p className="m-0 text-sm text-muted-foreground">
        Each workspace has its own tasks, records, agents, connectors and
        computers, which only its members reach. A person has a computer in each
        workspace they are in.
      </p>
      <div data-testid="workspaces">
        {(page.Workspaces ?? []).map((space) => (
          <Space key={space.ID} space={space} admin={page.IsAdmin} />
        ))}
      </div>
      {page.IsAdmin && (
        <form method="post" action="/org/workspaces" className="flex gap-2">
          <Input
            className="min-w-0 flex-1"
            name="name"
            placeholder="Engineering"
            aria-label="Name of a new workspace"
            maxLength={60}
            required
            data-testid="new-workspace"
          />
          <Button variant="outline" type="submit" data-testid="make-workspace">
            Make a workspace
          </Button>
        </form>
      )}
    </Section>
  )
}

function Space({ space, admin }: { space: CloudSpace; admin: boolean }) {
  const others = space.Others ?? []
  return (
    <div className="border-b py-3 last:border-b-0">
      {admin ? (
        <form
          method="post"
          action="/org/workspaces/rename"
          className="flex gap-2"
        >
          <input type="hidden" name="workspace" value={space.ID} />
          <Input
            className="min-w-0 flex-1 font-medium"
            name="name"
            defaultValue={space.Name}
            aria-label={`Name of the workspace ${space.Name}`}
            maxLength={60}
            required
          />
          <Button variant="outline" size="sm" type="submit">
            Rename
          </Button>
        </form>
      ) : (
        <p className="m-0 text-sm font-medium">{space.Name}</p>
      )}
      <ul className="m-0 mt-2 grid list-none p-0 text-sm">
        {(space.People ?? []).map((person) => (
          <li key={person.Email} className="flex min-h-10 items-center gap-3">
            <span className="flex-1 wrap-anywhere text-muted-foreground">
              {person.Email}
            </span>
            {admin && person.Removable && (
              <PersonForm
                action="/org/workspaces/remove"
                fields={{ workspace: space.ID, email: person.Email }}
              >
                Remove from workspace
              </PersonForm>
            )}
          </li>
        ))}
      </ul>
      {admin && others.length > 0 && (
        <form
          method="post"
          action="/org/workspaces/add"
          className="mt-2 flex gap-2"
        >
          <input type="hidden" name="workspace" value={space.ID} />
          <FormSelect
            className="min-w-0 flex-1"
            name="email"
            label={`Person to add to ${space.Name}`}
            groups={[
              {
                choices: others.map((email) => ({
                  value: email,
                  label: email,
                })),
              },
            ]}
            value={others[0]}
          />
          <Button variant="outline" size="sm" type="submit">
            Add
          </Button>
        </form>
      )}
    </div>
  )
}
