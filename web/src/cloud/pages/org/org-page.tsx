import { SignOut } from "@/cloud/components/cloud-card"
import { Section } from "@/cloud/pages/org/org-parts"
import { OrgPeople } from "@/cloud/pages/org/org-people"
import { OrgWorkspaces } from "@/cloud/pages/org/org-workspaces"
import type { CloudPage } from "@/cloud/types"
import { Figure, Figures } from "@/components/figures"
import { Button, buttonVariants } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { showsWorkspaces } from "@/domain/cloud-org"
import { cn } from "@/lib/utils"

// OrgPage is an organization as gusto cloud keeps it: what it may do, its details and plan, its people and its workspaces, in
// the admin layout with the organization's name down the side.
export function OrgPage({ page }: { page: CloudPage }) {
  const org = page.Org
  if (!org) return null
  return (
    <div className="grid min-h-dvh grid-cols-[15.5rem_minmax(0,1fr)] max-nav:grid-cols-[minmax(0,1fr)] max-nav:grid-rows-[auto_minmax(0,1fr)]">
      <OrgSide name={org.Name} several={page.Several} />
      <main className="min-w-0">
        <div className="mx-auto grid max-w-[60rem] grid-cols-[minmax(0,1fr)] gap-4 px-8 pt-8 pb-20 max-nav:px-4 max-nav:pt-5">
          <div className="flex min-h-10 flex-wrap items-center gap-x-3 gap-y-2">
            <h1 className="m-0 text-2xl font-semibold" data-testid="org-name">
              {org.Name}
            </h1>
            <span className="flex-1" />
            {page.CanOpen && (
              <a
                className={buttonVariants()}
                href={`/open?org=${encodeURIComponent(org.Slug)}`}
                data-testid="open-gusto"
              >
                Open gusto
              </a>
            )}
          </div>
          {page.Notice && (
            <p
              className="m-0 rounded-md bg-secondary px-4 py-3 text-sm"
              role="status"
            >
              {page.Notice}
            </p>
          )}
          <OrgFigures page={page} />
          <OrgDetails page={page} />
          <OrgPeople page={page} />
          {showsWorkspaces(page, page.Workspaces?.length ?? 0) && (
            <OrgWorkspaces page={page} />
          )}
          <OrgFoot page={page} />
        </div>
      </main>
    </div>
  )
}

function OrgSide({ name, several }: { name: string; several: boolean }) {
  return (
    <nav
      className="flex min-w-0 flex-col gap-1 border-r bg-sidebar px-3 py-4 max-nav:flex-row max-nav:flex-wrap max-nav:items-center max-nav:border-r-0 max-nav:border-b"
      aria-label="Organization"
    >
      <div className="flex min-h-12 items-center gap-3 px-3 max-nav:flex-1">
        <img className="w-8 flex-none" src="/static/mark.svg" alt="" />
        <span className="grid min-w-0 leading-tight">
          <strong className="truncate font-heading text-sm font-semibold">
            {name}
          </strong>
          <span className="text-xs text-muted-foreground">Organization</span>
        </span>
      </div>
      {several && (
        <a
          className="flex h-9 items-center rounded-md px-3 text-[0.8125rem] font-medium text-sidebar-foreground no-underline hover:bg-sidebar-accent hover:text-foreground"
          href="/orgs"
        >
          Switch organization
        </a>
      )}
      <span className="flex-1 max-nav:hidden" />
      <SignOut className="[&_button]:w-full [&_button]:justify-start" />
    </nav>
  )
}

function OrgFigures({ page }: { page: CloudPage }) {
  const quota = page.Quota
  return (
    <Figures className="mb-0">
      <Figure
        value={quota.Tasks}
        label={quota.Monthly ? "Tasks a month" : "Tasks"}
        detail={`${quota.Monthly ? "a month" : "in all"}, ${quota.Concurrent} at a time`}
        testId="figure-tasks"
      />
      <Figure
        value={`${page.Members?.length ?? 0} of ${quota.Members}`}
        label="People"
        detail={`up to ${quota.Members}`}
        testId="figure-people"
      />
      <Figure
        value={`${page.Computers} of ${quota.Computers}`}
        label="Computers"
        testId="figure-computers"
      />
      {page.Credit && (
        <Figure
          value={page.Credit.Left}
          label="Credit left"
          detail={`left of ${page.Credit.Of} on gusto's models`}
          testId="figure-credit"
        />
      )}
    </Figures>
  )
}

function Status({ ok, children }: { ok: boolean; children: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium",
        ok ? "bg-ok-wash text-ok" : "bg-warn-wash text-warn"
      )}
    >
      {children}
    </span>
  )
}

function OrgDetails({ page }: { page: CloudPage }) {
  const org = page.Org!
  return (
    <Section title="Details">
      <dl className="m-0 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-8 gap-y-2.5 text-sm [&>dd]:m-0 [&>dd]:wrap-anywhere [&>dt]:text-muted-foreground">
        <dt>Address</dt>
        <dd>{page.Address}</dd>
        {page.Domain && (
          <>
            <dt>Company</dt>
            <dd>{page.Domain}</dd>
          </>
        )}
        <dt>Admin</dt>
        <dd>{org.Admin}</dd>
        {!org.Demo && (
          <>
            <dt>Your email</dt>
            <dd data-testid="email-state">
              <Status ok={!page.Person.Trial}>
                {page.Person.Trial ? "Not verified" : "Verified"}
              </Status>
            </dd>
          </>
        )}
        {page.Plan && (
          <>
            <dt>Plan</dt>
            <dd>{page.Plan.Words}</dd>
          </>
        )}
        {page.Server && (
          <>
            <dt>Server</dt>
            <dd>
              <Status ok={page.CanOpen}>
                {page.CanOpen ? "Ready" : "Starting"}
              </Status>
            </dd>
          </>
        )}
        {page.Credit && page.IsAdmin && (
          <>
            <dt>Models</dt>
            <dd>
              When the free credit is spent, add your own key in Admin, Models.
            </dd>
          </>
        )}
      </dl>
      {page.Plan?.Subscribe && (
        <form method="post" action="/org/billing/subscribe">
          <Button type="submit" data-testid="subscribe">
            {page.Plan.Offer}
          </Button>
        </form>
      )}
      {page.Plan?.Manage && (
        <form method="post" action="/org/billing/manage">
          <Button variant="outline" type="submit" data-testid="manage-billing">
            Card, invoices and plan
          </Button>
        </form>
      )}
    </Section>
  )
}

function OrgFoot({ page }: { page: CloudPage }) {
  if (page.Org?.Demo) {
    return (
      <p className="m-0 text-sm text-muted-foreground">
        This is a demo. Its time left, and a call with us, are on{" "}
        <a className="underline hover:text-foreground" href="/demo/wait">
          the demo page
        </a>
        .
      </p>
    )
  }
  if (!page.Person.Trial) return null
  return (
    <Card className="py-5">
      <CardContent className="grid gap-3 px-5">
        <p className="m-0 text-sm text-muted-foreground">
          Confirm your email address to raise this to {page.Verified.Tasks}{" "}
          tasks a month and to invite colleagues. We mailed a link to{" "}
          {page.Person.Email}.
        </p>
        <form method="post" action="/org/verify">
          <Button variant="outline" type="submit" data-testid="send-link-again">
            Send the link again
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
