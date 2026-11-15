import { FolderIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"

// Folder is where a chat works: a folder of the cloud computer (at), or one of the person's own computer the desktop app synced
// (key and name). None is no folder.
export interface Folder {
  at: string
  key: string
  name: string
  label: string
}

export const noFolder: Folder = { at: "", key: "", name: "", label: "" }

// FolderChip names the folder a chat will work in and opens the window that chooses it; a chosen folder can be let go again.
export function FolderChip({
  folder,
  onOpen,
  onClear,
}: {
  folder: Folder
  onOpen: () => void
  onClear: () => void
}) {
  const chosen = Boolean(folder.label)
  return (
    <span className="flex min-w-0 items-center">
      <input type="hidden" name="at" value={folder.at} />
      <input type="hidden" name="folder" value={folder.key} />
      <input type="hidden" name="folder_name" value={folder.name} />
      <Button
        type="button"
        variant="secondary"
        size="sm"
        className="min-w-0 rounded-full text-xs font-medium text-muted-foreground"
        aria-haspopup="dialog"
        data-testid="folder-chip"
        title={chosen ? folder.label : undefined}
        onClick={onOpen}
      >
        <FolderIcon />
        <span className="max-w-56 truncate max-nav:max-w-24 max-[30rem]:sr-only">
          {chosen ? folder.label : "Choose a folder"}
        </span>
      </Button>
      {chosen && (
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="rounded-full text-subtle"
          aria-label="Do not use a folder"
          onClick={onClear}
        >
          <XIcon />
        </Button>
      )}
    </span>
  )
}
