import { useParams } from "react-router"

import { Loading } from "@/components/controls"
import { Notice, PageBar, PageBody } from "@/components/page"
import { Button } from "@/components/ui/button"
import { usePageData } from "@/lib/api"
import {
  ChangesCard,
  ShownVersionCard,
} from "@/pages/admin/apps/integration-shown"
import {
  IntegrationHistory,
  OnFor,
  Versions,
} from "@/pages/admin/apps/integration-versions"
import { OnBadge } from "@/pages/admin/apps/on-badge"
import type { Integration, IntegrationData } from "@/pages/admin/apps/types"
import { withBase } from "@/lib/base"

const crumbs = [{ label: "Apps & skills", href: "/admin/apps?tab=skills" }]

// One imported plugin: who it is on for, its versions and which is on, the version the address names, and what was done to it.
export function IntegrationPage() {
  const name = useParams().name ?? ""
  const loaded = usePageData<IntegrationData>()
  const held = loaded.state === "ready" ? loaded.data.Integration : undefined
  return (
    <>
      <PageBar
        crumbs={crumbs}
        title={
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate" title={name}>
              {name}
            </span>
            {held && <OnBadge on={held.On !== ""} />}
          </span>
        }
      >
        {held && <IntegrationActions held={held} />}
      </PageBar>
      <PageBody>
        <Loading loaded={loaded}>
          {(page) => <IntegrationParts page={page} />}
        </Loading>
      </PageBody>
    </>
  )
}

// IntegrationActions turn the plugin off, or put back the version that was on before.
function IntegrationActions({ held }: { held: Integration }) {
  const base = `/admin/integrations/${held.Name}`
  return (
    <div className="flex flex-none items-center gap-2">
      {held.On && (
        <form
          method="post"
          action={withBase(`${base}/disable`)}
          className="m-0"
        >
          <Button variant="outline" size="sm" type="submit">
            Turn off
          </Button>
        </form>
      )}
      {held.Before && (
        <form
          method="post"
          action={withBase(`${base}/restore`)}
          className="m-0"
        >
          <Button
            variant="outline"
            size="sm"
            type="submit"
            aria-label="Put the earlier version back"
          >
            <span className="max-sm:hidden">Put the earlier version back</span>
            <span className="sm:hidden">Earlier version</span>
          </Button>
        </form>
      )}
    </div>
  )
}

function IntegrationParts({ page }: { page: IntegrationData }) {
  const held = page.Integration
  return (
    <div className="grid gap-4">
      {page.Refused && <Notice>{page.Refused}</Notice>}
      <OnFor held={held} />
      <Versions held={held} />
      {page.Shown && <ShownVersionCard held={held} shown={page.Shown} />}
      {page.Shown && page.Changes && <ChangesCard changes={page.Changes} />}
      <IntegrationHistory history={held.History ?? []} />
    </div>
  )
}
