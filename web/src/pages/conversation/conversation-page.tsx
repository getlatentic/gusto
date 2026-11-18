import { useRef, useState } from "react"
import { useLocation } from "react-router"

import { AgentMark } from "@/components/controls"
import { Notice, PageBar } from "@/components/page"
import { pagePath, usePageData } from "@/lib/api"
import { api } from "@/store/api"
import { useShell } from "@/lib/shell"
import { AddDialog } from "@/pages/conversation/add-dialog"
import {
  ConversationComposer,
  type ComposerHandle,
} from "@/pages/conversation/conversation-composer"
import {
  ConversationContext,
  type Conversation,
} from "@/pages/conversation/conversation-context"
import { ConversationActions } from "@/pages/conversation/conversation-header"
import {
  FilePanel,
  roomFilesTab,
  type OpenFile,
} from "@/pages/conversation/file-panel"
import { GuidelinesDialog } from "@/pages/conversation/guidelines-dialog"
import { MembersDialog } from "@/pages/conversation/members-dialog"
import { useRoomDialog } from "@/pages/conversation/room-dialog"
import { TurnList } from "@/pages/conversation/turn-list"
import type { ConversationData } from "@/pages/conversation/types"
import { useRoomForm } from "@/pages/conversation/use-room-form"
import { useRoomLive } from "@/pages/conversation/use-room-live"
import { withBase } from "@/lib/base"

// ConversationPage is one conversation: with one agent, or a room of people and agents.
export function ConversationPage() {
  const where = useLocation()
  // A preview that is starting is looked at again every few seconds until it has.
  const cached = api.endpoints.page.useQueryState(pagePath(where)).data as
    ConversationData | undefined
  const loaded = usePageData<ConversationData>({
    pollingInterval: cached?.Starting ? 3_000 : 0,
  })
  if (loaded.state === "failed") {
    return (
      <>
        <PageBar title="Conversation" />
        <Notice>
          {loaded.status === 404
            ? "There is nothing here."
            : `This could not be read: ${loaded.message}`}
        </Notice>
      </>
    )
  }
  if (!loaded.data) return <PageBar title="Conversation" />
  return <Conversation key={loaded.data.Path} page={loaded.data} />
}

// wide is whether files open beside the conversation rather than in a page of their own.
const wide = () => window.matchMedia("(min-width: 72rem)").matches

function Conversation({ page }: { page: ConversationData }) {
  const admin = useShell().data?.Admin ?? false
  const live = useRoomLive(page)
  const forms = useRoomForm(page)
  const [dialog, showDialog] = useRoomDialog()
  const composer = useRef<ComposerHandle>(null)
  const [files, setFiles] = useState<OpenFile[]>([])
  const [current, setCurrent] = useState("")
  const openFile = (href: string, name: string) => {
    if (!wide()) return false
    setFiles((open) =>
      open.some((file) => file.href === href) ? open : [...open, { href, name }]
    )
    setCurrent(href)
    return true
  }
  const conversation: Conversation = {
    page,
    admin,
    answer: (name) => composer.current?.answer(name),
    openFile,
    // A reply is stopped over the socket when it is up, and by its form when it is not, or when the socket could not.
    stop: (ref, form) => {
      void live.stop(ref).then((stopped) => {
        if (!stopped) void forms.send(form)
      })
    },
    send: forms.send,
    submit: forms.submit,
    post: forms.post,
  }
  const direct = page.DirectAgent
  return (
    <ConversationContext.Provider value={conversation}>
      <div className="flex min-h-0 flex-1">
        <section
          aria-label={page.Named}
          className="grid min-h-0 min-w-0 flex-1 grid-rows-[auto_minmax(0,1fr)_auto]"
        >
          <PageBar
            title={page.Named}
            titleTestId="conversation-title"
            lead={
              direct && (
                <span
                  className="grid size-7 flex-none place-items-center overflow-hidden rounded-full bg-sunken"
                  aria-hidden="true"
                >
                  <AgentMark
                    mark={direct.Mark}
                    markDark={direct.MarkDark}
                    name={direct.Name}
                  />
                </span>
              )
            }
            crumbs={
              page.Instance.Room
                ? []
                : [
                    {
                      label: page.Agent.Name,
                      href: `/instances/${encodeURIComponent(page.Instance.ID)}`,
                    },
                  ]
            }
          >
            <ConversationActions
              here={live.here}
              onDialog={showDialog}
              onFiles={() => {
                if (!openFile(roomFilesTab, "Files"))
                  window.location.assign(
                    withBase(
                      `/instances/${encodeURIComponent(page.Instance.ID)}/files?from=${encodeURIComponent(page.Path)}`
                    )
                  )
              }}
            />
          </PageBar>
          <TurnList
            page={page}
            turns={live.turns.Turns ?? []}
            typing={live.typing}
            failed={forms.failed}
            onStarter={(said) => composer.current?.write(said)}
          />
          <div className="px-6 pt-2 pb-3 max-nav:px-3">
            <ConversationComposer page={page} live={live} handle={composer} />
          </div>
        </section>
        {files.length > 0 && (
          <FilePanel
            page={page}
            open={files}
            current={current}
            onShow={setCurrent}
            onOpen={openFile}
            onClose={(href) => {
              const left = files.filter((file) => file.href !== href)
              setFiles(left)
              if (href === current)
                setCurrent(left[left.length - 1]?.href ?? "")
            }}
            onCloseAll={() => {
              setFiles([])
              setCurrent("")
            }}
          />
        )}
      </div>
      {page.Instance.Room && (
        <>
          <MembersDialog
            open={dialog === "members"}
            onClose={() => showDialog(undefined)}
            onAdd={() => showDialog("add")}
          />
          <AddDialog
            open={dialog === "add"}
            onClose={() => showDialog(undefined)}
          />
          <GuidelinesDialog
            open={dialog === "guidelines"}
            onClose={() => showDialog(undefined)}
          />
        </>
      )}
    </ConversationContext.Provider>
  )
}
