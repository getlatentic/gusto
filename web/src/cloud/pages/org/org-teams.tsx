import { useState } from "react"

import {
  Row,
  RowMenu,
  Section,
  type RowAction,
} from "@/cloud/pages/org/org-parts"
import type { CloudPage, CloudSpace } from "@/cloud/types"
import { Field, FormDialog } from "@/components/form-dialog"
import { FormSelect } from "@/components/form-select"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

// OrgTeams is the organization's teams and who is in each. An admin makes them, and renames, fills and empties each
// from its ⋯.
export function OrgTeams({ page }: { page: CloudPage }) {
  return (
    <Section title="Teams" id="teams" action={page.IsAdmin && <NewTeam />}>
      <p className="m-0 text-sm text-muted-foreground">
        Each team has its own tasks, records, agents, connectors and computers,
        which only its members reach. A person has a computer in each team they
        are in.
      </p>
      <div className="grid gap-4" data-testid="teams">
        {(page.Teams ?? []).map((space) => (
          <Space key={space.ID} space={space} admin={page.IsAdmin} />
        ))}
      </div>
    </Section>
  )
}

function NewTeam() {
  return (
    <FormDialog
      trigger={<Button variant="outline" size="sm" data-testid="make-team" />}
      label="New team"
      title="New team"
      words="Its tasks, records, agents and computers are its own. You are its first member."
      action="/org/teams"
      submit="Make the team"
    >
      <Field id="team-name" label="Name">
        <Input
          id="team-name"
          name="name"
          placeholder="Engineering"
          maxLength={60}
          required
          data-testid="new-team"
        />
      </Field>
    </FormDialog>
  )
}

function Space({ space, admin }: { space: CloudSpace; admin: boolean }) {
  const [renaming, setRenaming] = useState(false)
  const [adding, setAdding] = useState(false)
  const others = space.Others ?? []
  const people = space.People ?? []
  const actions: RowAction[] = admin
    ? [
        { label: "Rename", onSelect: () => setRenaming(true) },
        ...(others.length > 0
          ? [{ label: "Add a person", onSelect: () => setAdding(true) }]
          : []),
      ]
    : []
  return (
    <section className="grid gap-1" aria-label={space.Name}>
      <div className="flex min-h-8 items-center gap-3">
        <h3 className="m-0 min-w-0 flex-1 truncate font-sans text-sm font-medium">
          {space.Name}
        </h3>
        <span className="text-sm text-muted-foreground">
          {people.length} {people.length === 1 ? "person" : "people"}
        </span>
        <span className="flex w-7 justify-end">
          <RowMenu label={space.Name} actions={actions} />
        </span>
      </div>
      <ul className="m-0 grid list-none border-t p-0 text-sm">
        {people.map((person) => (
          <Row
            key={person.Email}
            menu={
              admin &&
              person.Removable && (
                <RowMenu
                  label={person.Email}
                  actions={[
                    {
                      label: "Remove from team",
                      action: "/org/teams/remove",
                      fields: { team: space.ID, email: person.Email },
                      destructive: true,
                    },
                  ]}
                />
              )
            }
          >
            <span className="text-muted-foreground">{person.Email}</span>
          </Row>
        ))}
      </ul>
      {admin && (
        <>
          <FormDialog
            open={renaming}
            onOpenChange={setRenaming}
            title={`Rename ${space.Name}`}
            words="Its people, tasks and computers stay as they are."
            action="/org/teams/rename"
            submit="Rename"
          >
            <input type="hidden" name="team" value={space.ID} />
            <Field id={`rename-${space.ID}`} label="Name">
              <Input
                id={`rename-${space.ID}`}
                name="name"
                defaultValue={space.Name}
                maxLength={60}
                required
              />
            </Field>
          </FormDialog>
          {others.length > 0 && (
            <FormDialog
              open={adding}
              onOpenChange={setAdding}
              title={`Add a person to ${space.Name}`}
              words="They get a computer in this team too."
              action="/org/teams/add"
              submit="Add"
            >
              <input type="hidden" name="team" value={space.ID} />
              <Field id={`add-${space.ID}`} label="Person">
                <FormSelect
                  id={`add-${space.ID}`}
                  name="email"
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
              </Field>
            </FormDialog>
          )}
        </>
      )}
    </section>
  )
}
