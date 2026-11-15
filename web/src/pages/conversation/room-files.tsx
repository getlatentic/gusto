import { useState } from "react"
import { FileTextIcon, UploadIcon } from "lucide-react"

import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useData } from "@/lib/api"
import { useAppDispatch } from "@/store"
import { api } from "@/store/api"
import { bytes, shortWhen } from "@/lib/format"
import { fileHref } from "@/pages/conversation/file-chips"
import type {
  ConversationData,
  RoomFilesData,
} from "@/pages/conversation/types"

// RoomFiles is a room's files by who made them, newest day first, and the way for its people to add theirs.
export function RoomFiles({
  page,
  onOpen,
}: {
  page: ConversationData
  onOpen: (href: string, name: string) => void
}) {
  const path = `/app/api/rooms/${encodeURIComponent(page.Instance.ID)}/files`
  const dispatch = useAppDispatch()
  const files = useData<RoomFilesData>(path)
  const [by, setBy] = useState<"agents" | "people">("agents")
  // adding is what became of the last files added: on their way, refused with why, or nothing to say.
  const [adding, setAdding] = useState("")
  const upload = async (chosen: File[]) => {
    const body = new FormData()
    for (const file of chosen) body.append("file", file)
    setAdding("Adding…")
    const answer = await fetch(
      `/rooms/${encodeURIComponent(page.Instance.ID)}/files`,
      {
        method: "POST",
        credentials: "same-origin",
        body,
      }
    ).catch(() => null)
    if (!answer?.ok) {
      setAdding(
        (answer && (await answer.text()).trim()) ||
          "The files could not be added."
      )
      return
    }
    setAdding("")
    dispatch(api.util.invalidateTags([{ type: "Page", id: path }]))
    setBy("people")
  }
  const data = files.state === "failed" ? undefined : files.data
  if (!data)
    return (
      <p className="m-0 p-4 text-sm text-muted-foreground">
        {files.state === "failed" ? "The files could not be read." : "Looking…"}
      </p>
    )
  const days = (by === "agents" ? data.ByAgents : data.ByPeople) ?? []
  return (
    <div className="grid min-h-0 content-start gap-3 overflow-y-auto p-4">
      <div className="flex items-center gap-2">
        <Tabs
          value={by}
          onValueChange={(next) => setBy(next as "agents" | "people")}
          className="flex-1"
        >
          <TabsList aria-label="Whose files">
            <TabsTrigger value="agents">By agents {data.Agents}</TabsTrigger>
            <TabsTrigger value="people">By people {data.People}</TabsTrigger>
          </TabsList>
        </Tabs>
        {!data.Archived && (
          <label className="inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-md border px-3 text-xs font-medium hover:bg-muted has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring">
            <UploadIcon className="size-3.5" aria-hidden="true" /> Upload
            <input
              className="sr-only"
              type="file"
              multiple
              onChange={(event) => {
                void upload([...(event.currentTarget.files ?? [])])
                event.currentTarget.value = ""
              }}
            />
          </label>
        )}
      </div>
      <p
        role="status"
        className="m-0 text-xs text-muted-foreground empty:hidden"
      >
        {adding}
      </p>
      {days.length === 0 ? (
        <p className="m-0 text-sm text-muted-foreground">
          {by === "agents"
            ? "No agent has made a file here yet."
            : "Nobody has added a file here yet."}
        </p>
      ) : (
        days.map((day) => (
          <section key={day.Day} aria-label={day.Day} className="grid gap-1">
            <h3 className="m-0 text-xs font-medium text-muted-foreground">
              {day.Day}
            </h3>
            <ul className="m-0 grid list-none gap-px p-0">
              {day.Files.map((file) => {
                const href = fileHref(page.Instance.ID, file.Path, page.Path)
                return (
                  <li key={file.Path}>
                    <a
                      href={href}
                      title={file.Path}
                      className="flex min-h-9 items-center gap-2 rounded-md px-2 text-sm text-foreground no-underline hover:bg-muted"
                      onClick={(event) => {
                        event.preventDefault()
                        onOpen(href, file.Name)
                      }}
                    >
                      <FileTextIcon
                        aria-hidden="true"
                        className="size-4 flex-none text-subtle"
                      />
                      <span className="min-w-0 flex-1 truncate">
                        {file.Name}
                      </span>
                      <span className="flex-none text-xs text-subtle">
                        {bytes(file.Size)}
                      </span>
                      <span className="flex-none text-xs text-subtle">
                        {shortWhen(file.Modified)}
                      </span>
                    </a>
                  </li>
                )
              })}
            </ul>
          </section>
        ))
      )}
      {data.Cut && (
        <p className="m-0 text-xs text-subtle">
          Only the newest files are listed.
        </p>
      )}
      <a
        className="text-xs text-muted-foreground"
        href={`/instances/${encodeURIComponent(page.Instance.ID)}/files?from=${encodeURIComponent(page.Path)}`}
      >
        Open the folder
      </a>
    </div>
  )
}
