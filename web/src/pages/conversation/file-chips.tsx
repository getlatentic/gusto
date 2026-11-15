import {
  FileCodeIcon,
  FileIcon,
  FileImageIcon,
  FileTextIcon,
} from "lucide-react"

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { plural } from "@/lib/format"
import type { FileChip, TurnFiles } from "@/pages/conversation/types"

const looks = {
  image: FileImageIcon,
  code: FileCodeIcon,
  blank: FileIcon,
  doc: FileTextIcon,
}

// fileHref is where a file of the conversation's folder opens: in the file panel beside the conversation, on a wide window.
export const fileHref = (instance: string, path: string, from: string) =>
  `/instances/${encodeURIComponent(instance)}/file?path=${encodeURIComponent(path)}&from=${encodeURIComponent(from)}`

function Chip({
  chip,
  href,
  onOpen,
}: {
  chip: FileChip
  href: string
  onOpen: (href: string, name: string) => boolean
}) {
  const Look = looks[chip.Look as keyof typeof looks] ?? FileTextIcon
  return (
    <a
      href={href}
      title={chip.Path}
      data-testid="file-chip"
      className="inline-flex h-8 max-w-full min-w-0 items-center gap-2 rounded-md border border-input bg-surface px-3 text-sm text-foreground no-underline hover:bg-muted [&>svg]:size-4 [&>svg]:flex-none [&>svg]:text-subtle"
      onClick={(event) => {
        if (onOpen(href, chip.Name)) event.preventDefault()
      }}
    >
      <Look aria-hidden="true" />
      <span className="truncate">{chip.Name}</span>
    </a>
  )
}

// FileChips are the files a reply produced, each opening the file, past the first few folded away; a file no longer there is a
// line saying so.
export function FileChips({
  files,
  instance,
  from,
  onOpen,
}: {
  files: TurnFiles
  instance: string
  from: string
  onOpen: (href: string, name: string) => boolean
}) {
  const more = files.More ?? []
  const chip = (each: FileChip) => (
    <li key={each.Path}>
      <Chip
        chip={each}
        href={fileHref(instance, each.Path, from)}
        onOpen={onOpen}
      />
    </li>
  )
  return (
    <div className="col-start-2 mt-2 grid min-w-0 gap-2">
      <ul
        aria-label="Files from this reply"
        className="m-0 flex min-w-0 list-none flex-wrap gap-2 p-0"
      >
        {(files.Shown ?? []).map(chip)}
      </ul>
      {more.length > 0 && (
        <Collapsible>
          <CollapsibleTrigger className="cursor-pointer border-0 bg-transparent p-0 text-xs text-muted-foreground hover:text-foreground">
            Show {more.length} more {plural(more.length, "file")}
          </CollapsibleTrigger>
          <CollapsibleContent>
            <ul className="m-0 mt-2 flex min-w-0 list-none flex-wrap gap-2 p-0">
              {more.map(chip)}
            </ul>
          </CollapsibleContent>
        </Collapsible>
      )}
      {(files.Removed ?? []).map((gone) => (
        <p key={gone.Path} className="m-0 text-xs text-subtle">
          <span className="font-mono wrap-anywhere">{gone.Path}</span>{" "}
          {gone.Gone}
        </p>
      ))}
    </div>
  )
}
