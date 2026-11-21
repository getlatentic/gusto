import { RowMenu, Section, type RowAction } from "@/cloud/pages/org/org-parts"
import type { CloudBilling, CloudProposal } from "@/cloud/types"
import { Button, buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"

// OrgBilling is how the organization pays: the plan in words, the processor and the seats, the changes of seats proposed, and
// for an admin the ways to subscribe and, behind ⋯, to manage, cancel, and give an agent a token for proposing changes.
export function OrgBilling({ billing }: { billing: CloudBilling }) {
  return (
    <Section
      title="Billing"
      action={
        billing.Admin && (
          <RowMenu label="billing" actions={billingMenu(billing)} />
        )
      }
    >
      <dl className="m-0 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-8 gap-y-2.5 text-sm [&>dd]:m-0 [&>dd]:wrap-anywhere [&>dt]:text-muted-foreground">
        <dt>Plan</dt>
        <dd data-testid="plan">
          <span
            className={cn(
              "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium",
              billing.Active ? "bg-ok-wash text-ok" : "bg-warn-wash text-warn"
            )}
          >
            {billing.Words}
          </span>
        </dd>
        {billing.Provider && (
          <>
            <dt>Through</dt>
            <dd>{billing.Provider}</dd>
          </>
        )}
        {billing.Seats > 0 && (
          <>
            <dt>Seats</dt>
            <dd>
              {billing.Seats} paid, {billing.People} in use
            </dd>
          </>
        )}
      </dl>
      {(billing.Proposals ?? []).map((proposal) => (
        <Proposal
          key={proposal.ID}
          proposal={proposal}
          provider={billing.Provider}
        />
      ))}
      {billing.Admin && (billing.Offers ?? []).length > 0 && (
        <div className="flex flex-wrap gap-2">
          {(billing.Offers ?? []).map((offer) => (
            <form
              key={offer.Provider}
              method="post"
              action="/org/billing/subscribe"
            >
              <input type="hidden" name="provider" value={offer.Provider} />
              <Button size="sm" type="submit" data-testid="subscribe">
                {offer.Words}
              </Button>
            </form>
          ))}
        </div>
      )}
    </Section>
  )
}

function billingMenu(billing: CloudBilling): RowAction[] {
  const actions: RowAction[] = []
  if (billing.Manage) {
    actions.push({
      label: "Card, invoices and plan",
      action: "/org/billing/manage",
      fields: {},
    })
  }
  actions.push({
    label: "New agent token",
    action: "/org/billing/agent-token",
    fields: {},
  })
  if (billing.Cancel) {
    actions.push({
      label: "Cancel the subscription",
      action: "/org/billing/cancel",
      fields: {},
      destructive: true,
    })
  }
  return actions
}

// Proposal is one proposed change of seats, which an admin approves and pays for at the processor, or declines.
function Proposal({
  proposal,
  provider,
}: {
  proposal: CloudProposal
  provider: string
}) {
  return (
    <div
      className="grid gap-2 rounded-md border p-3 text-sm"
      data-testid="proposal"
    >
      <p className="m-0">
        {proposal.ByAgent ? "An agent" : "Someone"} proposes {proposal.Seats}{" "}
        seats. <span className="text-muted-foreground">{proposal.Reason}</span>
      </p>
      {proposal.Awaiting && (
        <p className="m-0 text-muted-foreground">
          Waiting for the payer to approve the change at {provider}.
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        {proposal.Pending && (
          <form method="post" action="/org/billing/proposals/approve">
            <input type="hidden" name="id" value={proposal.ID} />
            <Button size="sm" type="submit" data-testid="approve-proposal">
              Approve and go to {provider}
            </Button>
          </form>
        )}
        {proposal.Awaiting && proposal.ApproveURL && (
          <a
            className={buttonVariants({ size: "sm" })}
            href={proposal.ApproveURL}
          >
            Approve at {provider}
          </a>
        )}
        <form method="post" action="/org/billing/proposals/decline">
          <input type="hidden" name="id" value={proposal.ID} />
          <Button
            variant="outline"
            size="sm"
            type="submit"
            data-testid="decline-proposal"
          >
            Decline
          </Button>
        </form>
      </div>
    </div>
  )
}
