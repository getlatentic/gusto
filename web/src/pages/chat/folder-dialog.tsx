import { useReducer, type KeyboardEvent } from "react"
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  ChevronUpIcon,
  CloudIcon,
  FileIcon,
  FolderIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { NewFolderField } from "@/components/new-folder-field"
import {
  chosenPlace,
  emptyWords,
  explore,
  matching,
  nameAt,
  parentOf,
  startExplorer,
  step,
  type Explorer,
  type ShownFolders,
} from "@/domain/folders"
import { useData } from "@/lib/api"
import { postForm } from "@/lib/http"
import { useAppDispatch } from "@/store"
import { api } from "@/store/api"
import { bytes, shortWhen } from "@/lib/format"
import { cn } from "@/lib/utils"

const foldersAt = (at: string) => `/chat/folders?at=${encodeURIComponent(at)}`

// useFolders is what a place holds; what was shown stays while the next place is read.
function useFolders(at: string): { shown?: ShownFolders; failed: boolean } {
  const read = useData<ShownFolders>(foldersAt(at))
  return read.state === "failed"
    ? { failed: true }
    : { shown: read.data, failed: false }
}

// FolderDialog is the window that chooses the folder a chat works in: the person's projects down the side, the path along the
// top, the folder's contents, and a way to make a folder. The arrow keys move through the folders, Enter opens one, Backspace
// goes up, and Ctrl+Enter chooses.
export function FolderDialog({
  open,
  start,
  harness,
  onClose,
  onChoose,
}: {
  open: boolean
  start: string
  harness: string
  onClose: () => void
  onChoose: (shown: ShownFolders) => void
}) {
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent
        data-testid="folder-dialog"
        className="grid h-[min(34rem,100%)] w-[min(48rem,100%)] max-w-none grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-none"
      >
        <DialogHeader className="px-5 pt-4 pb-3">
          <DialogTitle>Choose a folder</DialogTitle>
        </DialogHeader>
        {open && (
          <Browser
            start={start}
            harness={harness}
            onClose={onClose}
            onChoose={onChoose}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function Browser({
  start,
  harness,
  onClose,
  onChoose,
}: {
  start: string
  harness: string
  onClose: () => void
  onChoose: (shown: ShownFolders) => void
}) {
  const [state, move] = useReducer(explore, start, startExplorer)
  const here = useFolders(state.at)
  const top = useFolders("")
  const dispatch = useAppDispatch()
  const choose = async () => {
    const place = chosenPlace(state)
    if (!place) return
    const read = await dispatch(
      api.endpoints.page.initiate(foldersAt(place), { subscribe: false })
    )
    if (read.data) onChoose(read.data as ShownFolders)
  }
  // make makes a folder here, then reads this place and the projects again, with the new folder chosen.
  const make = async (name: string) => {
    const form = new FormData()
    form.set("at", state.at)
    form.set("name", name)
    form.set("harness", harness)
    try {
      const made = await postForm<{ at: string }>("/chat/folders", form)
      dispatch(
        api.util.invalidateTags([
          { type: "Page", id: foldersAt(state.at) },
          { type: "Page", id: foldersAt("") },
        ])
      )
      move({ type: "made", at: made.at })
    } catch (err) {
      move({
        type: "failed",
        problem:
          (err instanceof Error && err.message) ||
          "That folder could not be made.",
      })
    }
  }
  const shown = here.shown
  const keys = (event: KeyboardEvent<HTMLElement>) => {
    if (!shown || (event.target as HTMLElement).closest("input, button, a"))
      return
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault()
      move({
        type: "select",
        at: step(
          shown,
          state.filter,
          state.selected,
          event.key === "ArrowDown" ? 1 : -1
        ),
      })
    } else if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
      event.preventDefault()
      void choose()
    } else if (event.key === "Enter" && state.selected) {
      event.preventDefault()
      move({ type: "open", at: state.selected })
    } else if (event.key === "Backspace" && shown.at) {
      event.preventDefault()
      move({ type: "open", at: parentOf(shown) })
    }
  }
  if (!shown) {
    return (
      <p className="m-0 p-8 text-center text-sm text-subtle">
        {here.failed ? "The folders could not be read." : "Looking…"}
      </p>
    )
  }
  return (
    <div
      className="grid min-h-0 min-w-0 grid-rows-[auto_minmax(0,1fr)_auto]"
      onKeyDown={keys}
    >
      <Toolbar shown={shown} state={state} move={move} />
      <div className="grid min-h-0 grid-cols-[12.5rem_minmax(0,1fr)] max-sm:grid-cols-[minmax(0,1fr)]">
        <Projects
          at={shown.at}
          projects={top.shown?.folders ?? []}
          move={move}
        />
        <Contents
          shown={shown}
          state={state}
          move={move}
          onMake={(name) => void make(name)}
        />
      </div>
      <DialogFooter className="m-0 grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-4 rounded-none px-4 py-3 max-sm:grid-cols-[minmax(0,1fr)_auto]">
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            data-testid="new-folder"
            onClick={() => move({ type: "make" })}
          >
            {shown.at ? "New Folder" : "New Project"}
          </Button>
          {desktop() && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => window.location.assign("/app/folder")}
            >
              Sync from this computer…
            </Button>
          )}
        </div>
        <p className="m-0 truncate text-right text-xs text-muted-foreground max-sm:hidden">
          {chosenPlace(state)
            ? `Work in ${nameAt(shown, chosenPlace(state))}`
            : "Open a project, or make one"}
        </p>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button
            size="sm"
            data-testid="choose-folder"
            disabled={!chosenPlace(state)}
            onClick={() => void choose()}
          >
            Choose
          </Button>
        </div>
      </DialogFooter>
    </div>
  )
}

// desktop is whether the page is in gusto's desktop app, which can sync a folder of the person's own computer.
const desktop = () =>
  Boolean((window as unknown as { latenticDesktop?: unknown }).latenticDesktop)

type Move = (move: Parameters<typeof explore>[1]) => void

function Toolbar({
  shown,
  state,
  move,
}: {
  shown: ShownFolders
  state: Explorer
  move: Move
}) {
  return (
    <div className="flex items-center gap-1 border-b px-4 pb-3">
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="Back"
        title="Back"
        disabled={state.back.length === 0}
        onClick={() => move({ type: "back" })}
      >
        <ChevronLeftIcon />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="Up one folder"
        title="Up one folder"
        disabled={!shown.at}
        onClick={() => move({ type: "open", at: parentOf(shown) })}
      >
        <ChevronUpIcon />
      </Button>
      <nav
        aria-label="Where this is"
        className="ml-2 flex h-8 min-w-0 flex-1 items-center gap-1 overflow-hidden rounded-sm bg-muted px-3 text-xs whitespace-nowrap"
      >
        <ol className="m-0 flex min-w-0 list-none items-center gap-1 p-0">
          {shown.crumbs.map((crumb, index) => {
            const last = index === shown.crumbs.length - 1
            return (
              <li key={crumb.at} className="flex min-w-0 items-center gap-1">
                {index > 0 && (
                  <span className="text-subtle" aria-hidden="true">
                    ›
                  </span>
                )}
                {last ? (
                  <span
                    aria-current="location"
                    className="truncate font-semibold"
                    title={crumb.name}
                  >
                    {crumb.name}
                  </span>
                ) : (
                  <button
                    type="button"
                    className="cursor-pointer truncate border-0 bg-transparent p-0 text-muted-foreground hover:text-foreground hover:underline"
                    onClick={() => move({ type: "open", at: crumb.at })}
                  >
                    {crumb.name}
                  </button>
                )}
              </li>
            )
          })}
        </ol>
      </nav>
      <Input
        type="search"
        placeholder="Filter"
        aria-label="Filter this folder"
        value={state.filter}
        onChange={(event) => move({ type: "filter", text: event.target.value })}
        className="ml-2 h-8 w-36 text-xs max-sm:hidden"
      />
    </div>
  )
}

const place =
  "flex w-full cursor-pointer items-center gap-2 rounded-sm border-0 bg-transparent p-2 text-left text-sm text-muted-foreground hover:bg-hover hover:text-foreground aria-[current=true]:bg-surface aria-[current=true]:text-foreground aria-[current=true]:shadow-sm [&>svg]:size-4 [&>svg]:flex-none"

function Projects({
  at,
  projects,
  move,
}: {
  at: string
  projects: { name: string; at: string }[]
  move: Move
}) {
  return (
    <nav
      aria-label="Projects"
      className="flex min-h-0 flex-col gap-px overflow-y-auto border-r bg-muted p-3 max-sm:hidden"
    >
      <button
        type="button"
        className={cn(place, "font-semibold text-foreground")}
        aria-current={!at || undefined}
        onClick={() => move({ type: "open", at: "" })}
      >
        <CloudIcon aria-hidden="true" />{" "}
        <span className="truncate">Your cloud computer</span>
      </button>
      {projects.slice(0, 14).map((project) => (
        <button
          key={project.at}
          type="button"
          className={cn(place, "pl-8")}
          aria-current={
            at === project.at || at.startsWith(`${project.at}/`) || undefined
          }
          title={project.name}
          onClick={() => move({ type: "open", at: project.at })}
        >
          <FolderIcon aria-hidden="true" />{" "}
          <span className="truncate">{project.name}</span>
        </button>
      ))}
    </nav>
  )
}

const row =
  "grid min-h-9 grid-cols-[minmax(0,1fr)_8.5rem_4.5rem] items-center gap-3 rounded-sm px-4 text-sm max-sm:grid-cols-[minmax(0,1fr)_4.5rem]"

function Contents({
  shown,
  state,
  move,
  onMake,
}: {
  shown: ShownFolders
  state: Explorer
  move: Move
  onMake: (name: string) => void
}) {
  const folders = matching(shown.folders, state.filter)
  const files = matching(shown.files, state.filter)
  const selected = state.selected || folders[0]?.at
  return (
    <section
      aria-label="In this folder"
      className="flex min-h-0 min-w-0 flex-col"
    >
      <div
        className={cn(
          row,
          "min-h-8 rounded-none border-b px-6 text-xs font-medium text-muted-foreground"
        )}
        aria-hidden="true"
      >
        <span>Name</span>
        <span className="max-sm:hidden">Modified</span>
        <span className="text-right">Size</span>
      </div>
      <div
        role="listbox"
        aria-label="Folders"
        className="min-h-0 flex-1 overflow-y-auto px-2 py-1"
      >
        {state.making && (
          <NewFolder
            top={!shown.at}
            problem={state.problem}
            onMake={onMake}
            onCancel={() => move({ type: "stopMaking" })}
          />
        )}
        {folders.map((folder) => (
          <div
            key={folder.at}
            role="option"
            aria-selected={state.selected === folder.at}
            tabIndex={folder.at === selected ? 0 : -1}
            className={cn(
              row,
              "cursor-pointer hover:bg-muted aria-selected:bg-muted aria-selected:outline-1 aria-selected:-outline-offset-1 aria-selected:outline-subtle"
            )}
            onClick={() => move({ type: "select", at: folder.at })}
            onDoubleClick={() => move({ type: "open", at: folder.at })}
          >
            <span className="flex min-w-0 items-center gap-3">
              <FolderIcon
                aria-hidden="true"
                className="size-4 flex-none text-muted-foreground"
              />
              <span className="truncate" title={folder.name}>
                {folder.name}
              </span>
            </span>
            <span className="text-xs whitespace-nowrap text-muted-foreground max-sm:hidden">
              {shortWhen(folder.modified)}
            </span>
            <ChevronRightIcon
              aria-hidden="true"
              className="size-4 justify-self-end text-subtle"
            />
          </div>
        ))}
        {files.map((file) => (
          <div
            key={file.name}
            role="option"
            aria-selected={false}
            aria-disabled="true"
            className={cn(row, "text-subtle")}
          >
            <span className="flex min-w-0 items-center gap-3">
              <FileIcon aria-hidden="true" className="size-4 flex-none" />
              <span className="truncate" title={file.name}>
                {file.name}
              </span>
            </span>
            <span className="text-xs whitespace-nowrap max-sm:hidden">
              {shortWhen(file.modified)}
            </span>
            <span className="text-right text-xs whitespace-nowrap">
              {bytes(file.size)}
            </span>
          </div>
        ))}
        {folders.length + files.length === 0 && !state.making && (
          <p className="m-0 px-4 py-12 text-center text-sm text-subtle">
            {emptyWords(shown, state.filter)}
          </p>
        )}
      </div>
    </section>
  )
}

// NewFolder names a folder to make here.
function NewFolder(props: {
  top: boolean
  problem: string
  onMake: (name: string) => void
  onCancel: () => void
}) {
  return (
    <div className="grid min-h-9 gap-1 px-4 py-1">
      <NewFolderField {...props} />
    </div>
  )
}
