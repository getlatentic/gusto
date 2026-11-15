import {
  Brand,
  BrandBar,
  CloudCard,
  Heading,
  Quiet,
  TextLink,
  Waiting,
} from "@/cloud/components/cloud-card"
import { OnceForm, Submit } from "@/cloud/components/forms"
import type { CloudPage } from "@/cloud/types"

// ConfirmPage is where a mailed link lands: one press confirms it, so a mail scanner that opens the link does not use it.
export function ConfirmPage({ page }: { page: CloudPage }) {
  return (
    <CloudCard>
      <Brand />
      <Heading>{page.Title}</Heading>
      {page.Inviter && <Quiet>{page.Inviter} invited you.</Quiet>}
      <OnceForm action={page.Action} button={page.Button} busy="Just a moment…">
        <input type="hidden" name="t" value={page.Token} />
      </OnceForm>
    </CloudCard>
  )
}

// OrgsPage is where a person in several organizations chooses which to open.
export function OrgsPage({ page }: { page: CloudPage }) {
  const orgs = page.Orgs ?? []
  return (
    <CloudCard>
      <BrandBar />
      <Heading>Choose an organization</Heading>
      <Quiet>
        {page.Email} is in {orgs.length} organizations.
      </Quiet>
      <ul className="m-0 grid list-none gap-2 p-0" data-testid="orgs">
        {orgs.map((org) => (
          <li
            key={org.Slug}
            className="flex items-center gap-3 rounded-md border hover:bg-accent"
          >
            <a
              className="grid flex-1 gap-1 p-3 no-underline"
              href={`/open?org=${encodeURIComponent(org.Slug)}`}
            >
              <span className="font-semibold wrap-anywhere">{org.Name}</span>
              <span className="text-sm wrap-anywhere text-muted-foreground">
                {org.Slug}.{page.Address}
              </span>
            </a>
            <TextLink
              className="p-3 text-sm text-muted-foreground"
              href={`/org?org=${encodeURIComponent(org.Slug)}`}
              aria-label={`${org.Name} settings`}
            >
              Settings
            </TextLink>
          </li>
        ))}
      </ul>
    </CloudCard>
  )
}

// StartingPage is shown while an organization's server starts; the page looks again by itself and goes in once it is up.
export function StartingPage({ page }: { page: CloudPage }) {
  return (
    <CloudCard>
      <BrandBar />
      <Heading>{page.Title}</Heading>
      <Waiting role="status">{page.Notice}</Waiting>
      <Quiet>
        You go in as soon as it is ready.{" "}
        <TextLink href="/org">Organization settings</TextLink>
      </Quiet>
    </CloudCard>
  )
}

// LeasePage is the host's lease: how long before it removes itself, and the way to keep it a while longer.
export function LeasePage({ page }: { page: CloudPage }) {
  const lease = page.Lease
  return (
    <CloudCard>
      <Brand />
      <Heading>{page.Title}</Heading>
      {page.Notice && <p className="m-0 text-sm text-bad">{page.Notice}</p>}
      {lease?.Left ? (
        <p className="m-0" data-testid="lease-left">
          It removes itself in <strong>{lease.Left}</strong>, at {lease.Ends}.
        </p>
      ) : (
        <p className="m-0">It is about to remove itself.</p>
      )}
      {lease?.CanExtend ? (
        <form className="grid" method="post" action="/lease/extend">
          <Submit data-testid="extend">Keep it for {lease.Step} more</Submit>
        </form>
      ) : (
        <Quiet>It cannot be kept any longer than this.</Quiet>
      )}
      <Quiet>
        Anything running is stopped when it goes, and the organizations and
        sign-ins on it go with it.
      </Quiet>
    </CloudCard>
  )
}
