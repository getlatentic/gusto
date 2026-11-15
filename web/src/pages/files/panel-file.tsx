import { useState } from "react"

import { Loading } from "@/components/controls"
import { withSource } from "@/domain/file-view"
import { dataOf, useData } from "@/lib/api"
import { FileCrumbs, FileView } from "@/pages/files/file-view"
import type { FileData } from "@/pages/files/types"

// PanelFile is a file beside a conversation: the file and what can be done with it, without the way back, which the conversation
// around it is. A web page's source is shown in its place. href is the file's own page.
export function PanelFile({ href }: { href: string }) {
  const [source, setSource] = useState(false)
  const address = source ? withSource(`/app/api${href}`) : `/app/api${href}`
  const loaded = useData<FileData>(address)
  const page = dataOf(loaded)
  return (
    <div
      className="min-h-0 flex-1 overflow-y-auto px-5 pb-8"
      data-testid="panel-file"
    >
      {page ? (
        <FileView
          key={address}
          page={{ ...page, Back: "" }}
          address={address}
          onSource={() => setSource(true)}
          frame={(actions, file) => (
            <>
              <header className="sticky top-0 z-10 -mx-5 mb-4 flex min-h-14 flex-wrap items-center gap-x-3 gap-y-2 bg-surface/92 px-5 py-2 backdrop-blur-xl">
                <div className="flex min-w-0 flex-1 items-center gap-1.5 text-sm">
                  <FileCrumbs crumbs={page.Crumbs} />
                  <h2
                    className="m-0 min-w-0 truncate font-sans text-sm font-semibold"
                    title={page.File.Name}
                  >
                    {page.File.Name}
                  </h2>
                </div>
                {actions}
              </header>
              {file}
            </>
          )}
        />
      ) : (
        <div className="pt-4">
          <Loading loaded={loaded}>{() => null}</Loading>
        </div>
      )}
    </div>
  )
}
