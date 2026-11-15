import {
  ChevronRightIcon,
  FileCodeIcon,
  FileSpreadsheetIcon,
  FileTextIcon,
  FolderIcon,
  ImageIcon,
} from "lucide-react"

import { AppLink } from "@/components/app-link"
import { NewFolderField } from "@/components/new-folder-field"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  emptyWords,
  fileKindOf,
  filesHref,
  matching,
  viewerHref,
  type Explorer,
  type ShownFolders,
} from "@/domain/folders"
import { bytes, shortWhen } from "@/lib/format"

const icons = {
  image: ImageIcon,
  table: FileSpreadsheetIcon,
  code: FileCodeIcon,
  file: FileTextIcon,
}

// A whole row opens what it names: its link is stretched over it.
const row = "relative h-12 hover:bg-muted/50"
const opens =
  "flex min-w-0 items-center gap-3 font-medium text-foreground no-underline after:absolute after:inset-0 after:content-[''] focus-visible:outline-none focus-visible:after:ring-2 focus-visible:after:ring-ring focus-visible:after:ring-inset [&>svg]:size-5 [&>svg]:flex-none"

// FolderList is what the place open holds, folders first, each with when it changed and how big it is. A folder being made is named
// at the top of the list.
export function FolderList({
  shown,
  state,
  onMake,
  onStopMaking,
}: {
  shown: ShownFolders
  state: Explorer
  onMake: (name: string) => void
  onStopMaking: () => void
}) {
  const folders = matching(shown.folders, state.filter)
  const files = matching(shown.files, state.filter)
  return (
    <section
      aria-label="In this folder"
      className="overflow-hidden rounded-lg bg-surface shadow-sm dark:border"
    >
      <Table className="table-fixed" data-testid="files-list">
        <TableHeader className="bg-muted">
          <TableRow className="hover:bg-transparent">
            <TableHead className="px-5">Name</TableHead>
            <TableHead className="w-32 max-sm:hidden">Modified</TableHead>
            <TableHead className="w-24 pr-5 text-right">Size</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {state.making && (
            <NewFolder
              top={!shown.at}
              problem={state.problem}
              onMake={onMake}
              onCancel={onStopMaking}
            />
          )}
          {folders.map((folder) => (
            <TableRow key={folder.at} className={row} data-testid="folder-row">
              <TableCell className="px-5">
                <AppLink href={filesHref(folder.at)} className={opens}>
                  <FolderIcon
                    aria-hidden="true"
                    className="text-muted-foreground"
                  />
                  <span className="truncate" title={folder.name}>
                    {folder.name}
                  </span>
                </AppLink>
              </TableCell>
              <TableCell className="text-xs text-muted-foreground max-sm:hidden">
                {shortWhen(folder.modified)}
              </TableCell>
              <TableCell className="pr-5 text-right">
                <ChevronRightIcon
                  aria-hidden="true"
                  className="ml-auto size-4 text-subtle"
                />
              </TableCell>
            </TableRow>
          ))}
          {files.map((file) => {
            const Icon = icons[fileKindOf(file.name)]
            return (
              <TableRow key={file.name} className={row} data-testid="file-row">
                <TableCell className="px-5">
                  <AppLink
                    href={viewerHref(shown.at, file.name)}
                    className={opens}
                  >
                    <Icon aria-hidden="true" className="text-subtle" />
                    <span className="truncate" title={file.name}>
                      {file.name}
                    </span>
                  </AppLink>
                </TableCell>
                <TableCell className="text-xs text-muted-foreground max-sm:hidden">
                  {shortWhen(file.modified)}
                </TableCell>
                <TableCell className="pr-5 text-right text-xs text-muted-foreground">
                  {bytes(file.size)}
                </TableCell>
              </TableRow>
            )
          })}
          {folders.length + files.length === 0 && !state.making && (
            <TableRow className="hover:bg-transparent">
              <TableCell
                colSpan={3}
                className="px-4 py-12 text-center whitespace-normal text-subtle"
              >
                {emptyWords(shown, state.filter)}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </section>
  )
}

// NewFolder names a folder to make here, at the top of the list.
function NewFolder(props: {
  top: boolean
  problem: string
  onMake: (name: string) => void
  onCancel: () => void
}) {
  return (
    <TableRow className="hover:bg-transparent">
      <TableCell colSpan={3} className="px-5 py-2 whitespace-normal">
        <div className="grid gap-1">
          <NewFolderField {...props} large />
        </div>
      </TableCell>
    </TableRow>
  )
}
