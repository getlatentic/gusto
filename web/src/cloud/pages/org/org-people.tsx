import {
  Row,
  RowMenu,
  Section,
  type RowAction,
} from "@/cloud/pages/org/org-parts"
import type { CloudMember, CloudPage } from "@/cloud/types"
import { Field, FormDialog } from "@/components/form-dialog"
import { FormSelect } from "@/components/form-select"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { memberActions } from "@/domain/cloud-org"

// OrgPeople is who is in the organization and who has been invited, with what an admin may change about each behind ⋯, and the
// way to invite one more.
export function OrgPeople({ page }: { page: CloudPage }) {
  return (
    <Section
      title="People"
      action={page.CanInvite && <InviteDialog page={page} />}
    >
      <ul className="m-0 grid list-none p-0 text-sm" data-testid="people">
        {(page.Members ?? []).map((member) => (
          <Row
            key={member.Email}
            note={`${member.Role}${member.Computer ? `, computer ${member.Computer}` : ""}`}
            menu={
              <RowMenu
                label={member.Email}
                actions={memberMenu(member, page)}
              />
            }
          >
            {member.Email}
          </Row>
        ))}
        {(page.Invited ?? []).map((email) => (
          <Row
            key={email}
            note={<Badge variant="secondary">invited</Badge>}
            menu={
              page.IsAdmin && (
                <RowMenu
                  label={email}
                  actions={[
                    {
                      label: "Cancel the invitation",
                      action: "/org/invites/cancel",
                      fields: { email },
                      destructive: true,
                    },
                  ]}
                />
              )
            }
          >
            {email}
          </Row>
        ))}
      </ul>
    </Section>
  )
}

function memberMenu(member: CloudMember, page: CloudPage): RowAction[] {
  const can = memberActions(member, page)
  const actions: RowAction[] = []
  const role = (to: string) => ({
    action: "/org/people/role",
    fields: { email: member.Email, role: to },
  })
  if (can.makeAdmin) actions.push({ label: "Make admin", ...role("admin") })
  if (can.makeRunner) actions.push({ label: "Make runner", ...role("runner") })
  if (can.remove) {
    actions.push({
      label: "Remove",
      action: "/org/people/remove",
      fields: { email: member.Email },
      destructive: true,
    })
  }
  return actions
}

function InviteDialog({ page }: { page: CloudPage }) {
  const spaces = page.Workspaces ?? []
  return (
    <FormDialog
      trigger={<Button variant="outline" size="sm" data-testid="invite" />}
      label="Invite"
      title="Invite a person"
      words="They are mailed a link that works once, for seven days."
      action="/org/invite"
      submit="Send the invitation"
    >
      <Field id="invite-email" label="Email">
        <Input
          id="invite-email"
          type="email"
          name="email"
          placeholder="colleague@company.com"
          autoComplete="off"
          required
          data-testid="invite-email"
        />
      </Field>
      {spaces.length > 1 && (
        <Field id="invite-workspace" label="Workspace they join">
          <FormSelect
            id="invite-workspace"
            name="workspace"
            groups={[
              {
                choices: spaces.map((space) => ({
                  value: space.ID,
                  label: space.Name,
                })),
              },
            ]}
            value={spaces[0].ID}
          />
        </Field>
      )}
    </FormDialog>
  )
}
