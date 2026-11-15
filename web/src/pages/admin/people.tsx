import { useRef } from "react"

import { LinkTabs, Loading, Pager } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { buttonVariants } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { useData, usePageData } from "@/lib/api"
import { ago, initial, usd, when } from "@/lib/format"
import type { ConsoleShell, PeopleData, PersonRow } from "@/lib/types"

// Who is in the organization, what each may do, and what each spent this month.
export function PeoplePage() {
  const loaded = usePageData<PeopleData>()
  const shell = useData<ConsoleShell>("/app/api/console")
  const org = shell.state === "ready" ? shell.data.orgName : undefined
  const manage = loaded.state === "ready" ? loaded.data.Manage : undefined
  return (
    <>
      <PageBar title="People">
        {manage && (
          <a className={buttonVariants()} href={manage}>
            Invite people
          </a>
        )}
      </PageBar>
      <PageBody>
        <PageSub>
          Who is in {org || "this organization"}, and what they may do
        </PageSub>
        <Loading loaded={loaded}>{(page) => <People page={page} />}</Loading>
      </PageBody>
    </>
  )
}

function People({ page }: { page: PeopleData }) {
  return (
    <>
      {page.Tabs && page.Tabs.length > 1 && (
        <LinkTabs
          label="Views"
          current={page.Tab ?? "people"}
          tabs={page.Tabs.map((tab) => ({
            key: tab.Key,
            href: tab.Href,
            label: (
              <>
                {tab.Count > 0 ? `${tab.Label} ` : tab.Label}
                {tab.Count > 0 && (
                  <span className="font-normal text-muted-foreground">
                    {tab.Count}
                  </span>
                )}
              </>
            ),
          }))}
        />
      )}
      {page.Refused && <Notice>{page.Refused}</Notice>}
      {page.Unreadable ? (
        <Notice>
          The list of people could not be read, so none is shown.{" "}
          {page.Unreadable}
        </Notice>
      ) : (
        <>
          {page.SpendUnreadable && (
            <Notice>
              Spending could not be read, so none is shown.{" "}
              {page.SpendUnreadable}
            </Notice>
          )}
          {page.Tab === "invited" ? (
            <Invited page={page} />
          ) : (
            <Members page={page} />
          )}
        </>
      )}
    </>
  )
}

function Invited({ page }: { page: PeopleData }) {
  return (
    <Card className="@container gap-0 overflow-hidden py-0">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Invited</TableHead>
            <TableHead className="@max-md:hidden">By</TableHead>
            <TableHead className="text-right">Link works until</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {page.Invited?.map((invite) => (
            <TableRow key={invite.Email}>
              <TableCell className="w-full max-w-0 truncate">
                {invite.Email}
              </TableCell>
              <TableCell className="text-muted-foreground @max-md:hidden">
                {invite.By}
              </TableCell>
              <TableCell className="text-right whitespace-nowrap text-muted-foreground">
                {invite.Expired ? "Expired " : ""}
                {when(invite.Expires)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}

function Members({ page }: { page: PeopleData }) {
  return (
    <>
      <Card className="@container gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Person</TableHead>
              <TableHead>Role</TableHead>
              <TableHead className="text-right">
                {page.Budgets ? (
                  <a
                    href="/admin/budgets"
                    className="text-inherit no-underline hover:underline"
                  >
                    Spent this month
                  </a>
                ) : (
                  "Spent this month"
                )}
              </TableHead>
              <TableHead className="text-right @max-lg:hidden">
                Last in
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {page.People?.map((person) => (
              <TableRow key={person.Email}>
                <TableCell className="w-full max-w-0">
                  <span className="flex min-w-0 items-center gap-3">
                    <span
                      className="grid size-8 flex-none place-items-center rounded-full border bg-muted text-xs font-semibold text-muted-foreground"
                      aria-hidden="true"
                    >
                      {initial(person.Name)}
                    </span>
                    <span className="grid min-w-0">
                      <span className="truncate">{person.Name}</span>
                      <span className="truncate text-xs text-muted-foreground">
                        {person.Email}
                      </span>
                    </span>
                  </span>
                </TableCell>
                <TableCell>
                  <Role person={person} page={page} />
                </TableCell>
                <TableCell className="text-right whitespace-nowrap tabular-nums">
                  <Spent person={person} />
                </TableCell>
                <TableCell className="text-right whitespace-nowrap text-muted-foreground @max-lg:hidden">
                  {person.LastSignIn ? (
                    <span title={when(person.LastSignIn)}>
                      {ago(person.LastSignIn)}
                    </span>
                  ) : (
                    "never"
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <Pager
          newer={page.Pages?.Previous}
          older={page.Pages?.Next}
          newerLabel="Previous"
          olderLabel="Next"
          shown={page.Pages?.Shown}
        />
      </Card>
      {page.Tokens && (
        <p className="mt-3 text-sm text-muted-foreground">
          Scripts and the command line sign in with{" "}
          <a
            className="font-medium text-foreground underline decoration-input underline-offset-2 hover:decoration-current"
            href="/admin/tokens"
          >
            API tokens
          </a>
          .
        </p>
      )}
    </>
  )
}

function Spent({ person }: { person: PersonRow }) {
  const spent = person.Spent
  return (
    <>
      {!spent ? (
        <span className="text-muted-foreground">nothing</span>
      ) : spent.unmeasured === 0 ? (
        usd(spent.usd)
      ) : spent.calls === spent.unmeasured ? (
        <span className="text-muted-foreground">no price</span>
      ) : (
        <>
          {usd(spent.usd)}{" "}
          <span className="text-muted-foreground">at least</span>
        </>
      )}
      {spent && person.Budget.Set && (
        <span className="text-muted-foreground">
          {" "}
          of {usd(person.Budget.Limit)}
        </span>
      )}
      {person.Budget.Passed && (
        <span className="text-xs font-medium text-bad"> reached</span>
      )}
    </>
  )
}

const roles = [
  { value: "admin", label: "Admin" },
  { value: "runner", label: "Runner" },
]

// Role is a person's role: a choice sent to gusto cloud, which owns roles, when there is one, and the role in words otherwise.
// The only admin is not offered the choice: an organization keeps one.
function Role({ person, page }: { person: PersonRow; page: PeopleData }) {
  const form = useRef<HTMLFormElement>(null)
  const role = useRef<HTMLInputElement>(null)
  const current = person.Role === "admin" ? "admin" : "runner"
  if (
    !page.RoleAction ||
    (person.Role === "admin" && (page.Admins ?? 0) <= 1)
  ) {
    return <span className="capitalize">{person.Role}</span>
  }
  const choose = (chosen: string) => {
    if (chosen === current || !form.current || !role.current) return
    if (
      current === "admin" &&
      !window.confirm(
        `Make ${person.Name} a runner? They lose the admin pages and can no longer change budgets or roles.`
      )
    )
      return
    role.current.value = chosen
    form.current.submit()
  }
  return (
    <form ref={form} method="post" action={page.RoleAction} className="m-0">
      <input type="hidden" name="email" value={person.Email} />
      <input type="hidden" name="back" value={page.Back} />
      <input ref={role} type="hidden" name="role" defaultValue={current} />
      <Select
        items={roles}
        value={current}
        onValueChange={(chosen) => choose(String(chosen))}
      >
        <SelectTrigger size="sm" aria-label={`Role of ${person.Name}`}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {roles.map((each) => (
            <SelectItem key={each.value} value={each.value}>
              {each.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </form>
  )
}
