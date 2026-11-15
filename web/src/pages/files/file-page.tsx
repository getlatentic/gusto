import { useLocation } from "react-router"

import { Loading } from "@/components/controls"
import { PageBar, PageBody } from "@/components/page"
import { dataOf, pagePath, usePageData } from "@/lib/api"
import { FileView } from "@/pages/files/file-view"
import type { FileData } from "@/pages/files/types"

// FilePage is one file of an agent's folder, or of a run's record, as a page of its own, under the pages it is in.
export function FilePage() {
  const address = pagePath(useLocation())
  const loaded = usePageData<FileData>()
  const page = dataOf(loaded)
  if (!page) {
    return (
      <>
        <PageBar title="File" />
        <PageBody className="max-w-5xl">
          <Loading loaded={loaded}>{() => null}</Loading>
        </PageBody>
      </>
    )
  }
  return (
    <FileView
      key={address}
      page={page}
      address={address}
      frame={(actions, file) => (
        <>
          <PageBar title={page.File.Name} crumbs={page.Crumbs}>
            {actions}
          </PageBar>
          <PageBody className="max-w-5xl">{file}</PageBody>
        </>
      )}
    />
  )
}
