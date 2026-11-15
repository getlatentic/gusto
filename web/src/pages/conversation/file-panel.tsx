import type { PointerEvent } from "react"
import { XIcon } from "lucide-react"

import { usePanelWidth } from "@/components/panel-resize"
import { Button } from "@/components/ui/button"
import { filePanel } from "@/domain/panel-width"
import { cn } from "@/lib/utils"
import { RoomFiles } from "@/pages/conversation/room-files"
import type { ConversationData } from "@/pages/conversation/types"

export interface OpenFile {
  href: string
  name: string
}

// roomFilesTab is the room's own list of files, which is a tab of the panel like any file.
export const roomFilesTab = "room-files"

// FilePanel is the files open beside a conversation on a wide window, one tab each; a file is its own page, drawn in a frame.
export function FilePanel({
  page,
  open,
  current,
  onShow,
  onClose,
  onCloseAll,
  onOpen,
}: {
  page: ConversationData
  open: OpenFile[]
  current: string
  onShow: (href: string) => void
  onClose: (href: string) => void
  onCloseAll: () => void
  onOpen: (href: string, name: string) => void
}) {
  const { width, divider } = usePanelWidth(
    "gusto-files-w",
    filePanel,
    "left",
    (event: PointerEvent<HTMLElement>) => {
      const right =
        event.currentTarget.parentElement?.getBoundingClientRect().right ??
        window.innerWidth
      return right - event.clientX
    }
  )
  return (
    <aside
      aria-label="Files from this chat"
      data-testid="file-panel"
      className="relative flex min-h-0 min-w-0 flex-none flex-col border-l bg-surface max-[72rem]:hidden"
      style={{ width: `${width}rem` }}
    >
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize the files"
        aria-valuenow={width}
        tabIndex={0}
        className="absolute inset-y-0 -left-1 z-10 w-2 cursor-col-resize touch-none after:absolute after:inset-y-0 after:left-0.75 after:w-0.5 after:content-[''] hover:after:bg-input focus-visible:after:bg-ring"
        {...divider}
      />
      <div className="flex items-center gap-2 border-b px-3 py-2">
        <div
          role="tablist"
          aria-label="Open files"
          className="flex min-w-0 flex-1 gap-1 overflow-x-auto"
        >
          {open.map((file) => (
            <span
              key={file.href}
              className={cn(
                "flex h-8 flex-none items-center rounded-md text-sm",
                file.href === current ? "bg-muted" : "hover:bg-muted"
              )}
            >
              <button
                type="button"
                role="tab"
                aria-selected={file.href === current}
                className="max-w-48 cursor-pointer truncate border-0 bg-transparent px-3"
                onClick={() => onShow(file.href)}
              >
                {file.name}
              </button>
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={`Close ${file.name}`}
                onClick={() => onClose(file.href)}
              >
                <XIcon />
              </Button>
            </span>
          ))}
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label="Close the files"
          title="Close the files"
          onClick={onCloseAll}
        >
          <XIcon />
        </Button>
      </div>
      {current === roomFilesTab ? (
        <RoomFiles page={page} onOpen={onOpen} />
      ) : (
        <iframe
          key={current}
          data-testid="file-frame"
          title={open.find((file) => file.href === current)?.name ?? "File"}
          src={`${current}&embed=1`}
          className="min-h-0 w-full flex-1 border-0"
        />
      )}
    </aside>
  )
}
