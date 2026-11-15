import { LinkTabs, Loading } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { useData, usePageData } from "@/lib/api"
import type { ConsoleShell } from "@/lib/types"
import { AppsList } from "@/pages/admin/apps/apps-list"
import { SkillsList } from "@/pages/admin/apps/skills-list"
import type { AppsData } from "@/pages/admin/apps/types"

// What agents may use: the apps people connect, and the skills they are given, each a tab of its own.
export function AppsPage() {
  const loaded = usePageData<AppsData>()
  const shell = useData<ConsoleShell>("/app/api/console")
  const org = shell.state === "ready" ? shell.data.orgName : undefined
  return (
    <>
      <PageBar title="Apps & skills" />
      <PageBody>
        <PageSub>
          What agents may use, for everyone in {org || "this workspace"}
        </PageSub>
        <Loading loaded={loaded}>{(page) => <AppsTabs page={page} />}</Loading>
      </PageBody>
    </>
  )
}

function AppsTabs({ page }: { page: AppsData }) {
  return (
    <>
      {page.Tabs.length > 1 && (
        <LinkTabs
          label="Views"
          current={page.Tab}
          tabs={page.Tabs.map((tab) => ({
            key: tab.Key,
            href: tab.Href,
            label: tab.Label,
          }))}
        />
      )}
      {page.Refused && <Notice>{page.Refused}</Notice>}
      {page.Tab === "apps" ? (
        <AppsList apps={page.Apps ?? []} turnedOn={page.TurnedOn ?? ""} />
      ) : (
        <SkillsList page={page} />
      )}
    </>
  )
}
