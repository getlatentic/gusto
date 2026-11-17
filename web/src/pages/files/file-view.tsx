import { Fragment, useState, type ReactNode } from "react"
import { DownloadIcon, SquarePenIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { Button, buttonVariants } from "@/components/ui/button"
import { factsOf } from "@/domain/file-view"
import { useAppDispatch } from "@/store"
import { api } from "@/store/api"
import { FileBody } from "@/pages/files/file-body"
import { FileEditor, type Opened } from "@/pages/files/file-editor"
import type { FileData } from "@/pages/files/types"
import { withBase } from "@/lib/base"

const action = buttonVariants({ variant: "outline", size: "sm" })

// FileView is one file: what it is and what can be done with it, above the file drawn by its kind or opened to be changed. address
// is where its data was read, which a save marks stale so the file is read again as it is now. frame draws what can be done with
// the file and the file where they go: the page's own bar and column, or the file panel's.
export function FileView({
  page,
  address,
  onSource,
  frame,
}: {
  page: FileData
  address: string
  // onSource shows a web page's source in place, where its own address is not the one to go to.
  onSource?: () => void
  frame: (actions: ReactNode, file: ReactNode) => ReactNode
}) {
  const dispatch = useAppDispatch()
  const [opened, setOpened] = useState<Opened | null>(null)
  const file = page.File
  const actions = (
    <FileActions
      page={page}
      editing={opened !== null}
      onEdit={() => setOpened({ text: file.Source, seen: file.Hash })}
      onSource={onSource}
    />
  )
  return frame(
    actions,
    <div className="grid max-w-[60rem] min-w-0 gap-5" data-testid="file">
      <p className="m-0 text-xs text-subtle" data-testid="file-facts">
        {factsOf(file)}
      </p>
      {opened ? (
        <FileEditor
          name={file.Name}
          save={page.Save}
          opened={opened}
          onCancel={() => setOpened(null)}
          onSaved={() => {
            dispatch(api.util.invalidateTags([{ type: "Page", id: address }]))
            setOpened(null)
          }}
        />
      ) : (
        <FileBody page={page} />
      )}
    </div>
  )
}

// FileActions are the way back, a web page's source, Edit for a file that can be changed here, and Download.
function FileActions({
  page,
  editing,
  onEdit,
  onSource,
}: {
  page: FileData
  editing: boolean
  onEdit: () => void
  onSource?: () => void
}) {
  return (
    <div className="flex flex-none items-center gap-2">
      {page.Back && (
        <AppLink href={page.Back} className={action} data-testid="file-back">
          <SquarePenIcon aria-hidden="true" />
          <span className="max-sm:sr-only">
            {page.BackWord || "Back to the chat"}
          </span>
        </AppLink>
      )}
      {page.Source && (
        <AppLink
          href={page.Source}
          className={action}
          onClick={(event) => {
            if (!onSource) return
            event.preventDefault()
            onSource()
          }}
        >
          View source
        </AppLink>
      )}
      {page.Save && !editing && (
        <Button
          variant="outline"
          size="sm"
          onClick={onEdit}
          data-testid="file-edit-open"
        >
          Edit
        </Button>
      )}
      <a
        href={withBase(page.File.Download)}
        download
        className={action}
        data-testid="file-download"
      >
        <DownloadIcon aria-hidden="true" />
        <span className="max-sm:sr-only">Download</span>
      </a>
    </div>
  )
}

// FileCrumbs are the pages above a file, for a bar that is not the page's own.
export function FileCrumbs({ crumbs }: { crumbs: FileData["Crumbs"] }) {
  return (
    <nav aria-label="Where this is" className="contents">
      {crumbs.map((crumb) => (
        <Fragment key={crumb.href}>
          <AppLink
            href={crumb.href}
            className="max-w-32 min-w-0 flex-none truncate text-muted-foreground no-underline hover:text-foreground"
          >
            {crumb.label}
          </AppLink>
          <span className="flex-none text-faint" aria-hidden="true">
            /
          </span>
        </Fragment>
      ))}
    </nav>
  )
}
