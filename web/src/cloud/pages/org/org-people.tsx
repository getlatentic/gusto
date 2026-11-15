import { PersonForm, Section } from "@/cloud/pages/org/org-parts"
import type { CloudMember, CloudPage } from "@/cloud/types"
import { FormSelect } from "@/components/form-select"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { memberActions } from "@/domain/cloud-org"

// OrgPeople is who is in the organization and who has been invited, with what an admin may change about each, and the form that
// invites one more.
export function OrgPeople({ page }: { page: CloudPage }) {
  return (
    <Section title="People">
      <ul className="m-0 grid list-none p-0 text-sm" data-testid="people">
        {(page.Members ?? []).map((member) => (
          <Member key={member.Email} member={member} page={page} />
        ))}
        {(page.Invited ?? []).map((email) => (
          <li
            key={email}
            className="flex min-h-12 items-center gap-3 border-b py-2 last:border-b-0"
          >
            <span className="flex-1 wrap-anywhere">{email}</span>
            <span className="inline-flex items-center rounded-full bg-secondary px-2 py-0.5 text-xs font-medium text-muted-foreground">
              invited
            </span>
            {page.IsAdmin && (
              <PersonForm action="/org/invites/cancel" fields={{ email }}>
                Cancel
              </PersonForm>
            )}
          </li>
        ))}
      </ul>
      {page.CanInvite && <InviteForm page={page} />}
    </Section>
  )
}

function Member({ member, page }: { member: CloudMember; page: CloudPage }) {
  const can = memberActions(member, page)
  return (
    <li className="flex min-h-12 flex-wrap items-center gap-3 border-b py-2 last:border-b-0">
      <span className="flex-1 wrap-anywhere">{member.Email}</span>
      <span className="text-muted-foreground">
        {member.Role}
        {member.Computer && `, computer ${member.Computer}`}
      </span>
      {can.makeRunner && (
        <PersonForm
          action="/org/people/role"
          fields={{ email: member.Email, role: "runner" }}
        >
          Make runner
        </PersonForm>
      )}
      {can.makeAdmin && (
        <PersonForm
          action="/org/people/role"
          fields={{ email: member.Email, role: "admin" }}
        >
          Make admin
        </PersonForm>
      )}
      {can.remove && (
        <PersonForm
          action="/org/people/remove"
          fields={{ email: member.Email }}
        >
          Remove
        </PersonForm>
      )}
    </li>
  )
}

function InviteForm({ page }: { page: CloudPage }) {
  const spaces = page.Workspaces ?? []
  return (
    <form method="post" action="/org/invite" className="flex flex-wrap gap-2">
      <Input
        className="min-w-0 flex-1"
        type="email"
        name="email"
        placeholder="colleague@company.com"
        aria-label="Email address to invite"
        autoComplete="off"
        required
        data-testid="invite-email"
      />
      {spaces.length > 1 && (
        <FormSelect
          name="workspace"
          label="Workspace they join"
          groups={[
            {
              choices: spaces.map((space) => ({
                value: space.ID,
                label: space.Name,
              })),
            },
          ]}
          value={spaces[0].ID}
          testId="invite-workspace"
        />
      )}
      <Button variant="outline" type="submit" data-testid="invite">
        Invite
      </Button>
    </form>
  )
}
