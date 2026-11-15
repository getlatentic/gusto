import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { Card } from "@/components/ui/card"
import { usePageData } from "@/lib/api"
import { ConnectionsTable } from "@/pages/account/connections-table"
import { HomeSection } from "@/pages/account/home-section"
import type { ConnectionsData } from "@/pages/account/types"
import { useAccountForm } from "@/pages/account/use-account-form"

// ConnectionsPage is the apps a person's agents use as them: each one they sign in to with their own account, whether their
// agents may use it, and the home their agents keep between tasks. Coming back from signing in at an app, it says so.
export function ConnectionsPage() {
  const loaded = usePageData<ConnectionsData>()
  return (
    <>
      <PageBar title="Connections" />
      <PageBody className="max-w-4xl">
        <PageSub>Apps your agents use as you</PageSub>
        <Loading loaded={loaded}>
          {(page) => <Connections page={page} />}
        </Loading>
      </PageBody>
    </>
  )
}

function Connections({ page }: { page: ConnectionsData }) {
  const form = useAccountForm()
  const refused = form.failed || page.Refused
  return (
    <>
      {refused && <Notice>{refused}</Notice>}
      {page.Done && (
        <Notice tone="plain">
          You are signed in to {page.Done}. Your agents use it from their next
          task.
        </Notice>
      )}
      {!page.Person && (
        <Notice tone="warn">
          You are signed in with the shared password, which is nobody's own.
          Sign in as yourself to connect your accounts.
        </Notice>
      )}
      {page.Connectors.length > 0 ? (
        <>
          <p className="m-0 mb-4 max-w-prose text-sm text-muted-foreground">
            Sign in with your own account. Your agents then reach what you can
            see in the app, and nothing more, and never hold your sign-in. In a
            room, agents use it only for what you ask, once you let them in that
            room.
          </p>
          <Card className="@container gap-0 overflow-hidden py-0">
            <ConnectionsTable apps={page.Connectors} form={form} />
          </Card>
        </>
      ) : (
        <p
          className="m-0 text-sm text-muted-foreground"
          data-testid="no-connections"
        >
          No app is turned on yet. An admin turns them on under Admin,
          Connectors.
        </p>
      )}
      {page.Home && (
        <HomeSection home={page.Home} reset={page.HomeReset} form={form} />
      )}
    </>
  )
}
