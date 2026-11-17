import { DownloadIcon, EllipsisIcon } from "lucide-react"

import { LinkTabs, Loading } from "@/components/controls"
import { Notice, PageBar, PageBody, PageSub } from "@/components/page"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { usePageData } from "@/lib/api"
import { ActivityTab } from "@/pages/admin/records/activity-tab"
import { ChecksTab } from "@/pages/admin/records/checks-tab"
import { HealthLine } from "@/pages/admin/records/health-line"
import { KeepingTab } from "@/pages/admin/records/keeping-tab"
import type { RecordsData } from "@/pages/admin/records/types"
import { withBase } from "@/lib/base"

// A signed record of everything that happened, whether the finished runs still check out, and how long work is kept.
export function RecordsPage() {
  const loaded = usePageData<RecordsData>()
  const logged = loaded.state === "ready" && loaded.data.Logged
  return (
    <>
      <PageBar title="Records">{logged && <RecordMenu />}</PageBar>
      <PageBody>
        <PageSub>
          A signed record of everything that happened, and how long work is kept
        </PageSub>
        <Loading loaded={loaded}>{(page) => <Records page={page} />}</Loading>
      </PageBody>
    </>
  )
}

// RecordMenu holds the download of the whole log, a file and not a page of this app.
function RecordMenu() {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="outline" size="icon" aria-label="More" />}
      >
        <EllipsisIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem
          render={<a href={withBase("/admin/activity.jsonl")} />}
        >
          <DownloadIcon /> Download the record
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function Records({ page }: { page: RecordsData }) {
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
      <HealthLine health={page.Health} />
      <TabPart page={page} />
    </>
  )
}

function TabPart({ page }: { page: RecordsData }) {
  if (page.Activity) {
    return <ActivityTab activity={page.Activity} health={page.Health} />
  }
  if (page.Keeping) return <KeepingTab keeping={page.Keeping} />
  if (page.Checks) return <ChecksTab checks={page.Checks} />
  return null
}
