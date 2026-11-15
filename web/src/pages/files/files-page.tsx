import { useReducer } from "react"
import { PlusIcon, SquarePenIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { Loading } from "@/components/controls"
import { PageBar, PageBody } from "@/components/page"
import { Button, buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  chatHref,
  explore,
  startExplorer,
  trailOf,
  type ExplorerMove,
} from "@/domain/folders"
import { dataOf, usePageData, useRefreshAll } from "@/lib/api"
import { postForm } from "@/lib/http"
import { FolderList } from "@/pages/files/folder-list"
import type { FilesData } from "@/pages/files/types"

// FilesPage is the person's cloud computer: the folders and files of one place, with their dates and sizes. A folder opens on one
// click, a file opens in the viewer, and a folder can be the one to chat in. Each place has its own address.
export function FilesPage() {
  const loaded = usePageData<FilesData>()
  const page = dataOf(loaded)
  if (!page) {
    return (
      <>
        <PageBar title="Files" />
        <PageBody className="max-w-5xl">
          <Loading loaded={loaded}>{() => null}</Loading>
        </PageBody>
      </>
    )
  }
  return <Place key={page.Here.at} page={page} />
}

function Place({ page }: { page: FilesData }) {
  const here = page.Here
  const [state, move] = useReducer(explore, here.at, startExplorer)
  const refreshAll = useRefreshAll()
  const { crumbs, title } = trailOf(here)
  // make makes a folder here, then reads every page again: a project made at the top is also a conversation's place.
  const make = async (name: string) => {
    const form = new FormData()
    form.set("at", here.at)
    form.set("name", name)
    form.set("harness", "")
    try {
      const made = await postForm<{ at: string }>("/chat/folders", form)
      refreshAll()
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
  return (
    <>
      <PageBar title={title} crumbs={crumbs}>
        <PlaceTools
          at={here.at}
          back={page.Back}
          filter={state.filter}
          move={move}
        />
      </PageBar>
      <PageBody className="max-w-5xl">
        <FolderList
          shown={here}
          state={state}
          onMake={(name) => void make(name)}
          onStopMaking={() => move({ type: "stopMaking" })}
        />
      </PageBody>
    </>
  )
}

// PlaceTools are what can be done in the place open: filter it, make a folder in it, and chat in it.
function PlaceTools({
  at,
  back,
  filter,
  move,
}: {
  at: string
  back: string
  filter: string
  move: (move: ExplorerMove) => void
}) {
  return (
    <div className="flex flex-none items-center gap-2">
      {back && (
        <AppLink
          href={back}
          className={buttonVariants({ variant: "outline", size: "sm" })}
          data-testid="files-back"
        >
          <SquarePenIcon aria-hidden="true" />
          <span className="max-sm:sr-only">Back to the chat</span>
        </AppLink>
      )}
      <Input
        type="search"
        placeholder="Filter"
        aria-label="Filter this folder"
        data-testid="files-filter"
        value={filter}
        onChange={(event) => move({ type: "filter", text: event.target.value })}
        className="h-8 w-40 text-xs max-sm:hidden"
      />
      <Button
        variant="outline"
        size="sm"
        data-testid="new-folder"
        onClick={() => move({ type: "make" })}
      >
        <PlusIcon aria-hidden="true" />
        {at ? "New folder" : "New project"}
      </Button>
      {at && (
        <AppLink
          href={chatHref(at)}
          className={buttonVariants({ size: "sm" })}
          data-testid="chat-here"
        >
          Chat here
        </AppLink>
      )}
    </div>
  )
}
