import {
  useImperativeHandle,
  useRef,
  useState,
  type FormEvent,
  type Ref,
} from "react"
import { useNavigate } from "react-router"
import { PaperclipIcon, SquareIcon, XIcon } from "lucide-react"

import { Composer } from "@/components/chat/composer"
import { Button } from "@/components/ui/button"
import { attach, sizeWords } from "@/domain/live-room"
import { sendForm } from "@/lib/forms"
import { ChoiceSelect } from "@/pages/chat/choice-select"
import { MentionBox } from "@/pages/conversation/mention-box"
import type { ConversationData } from "@/pages/conversation/types"
import type { RoomLive } from "@/pages/conversation/use-room-live"
import { useRefreshAll } from "@/lib/api"
import { withBase } from "@/lib/base"

// ConversationComposer is where a person writes in a conversation. In a room, words alone go over the live socket when it is
// up; anything else (files, or no socket) is sent by the form, as is everything in a conversation with one agent.
// ComposerHandle is what the page may do to the box: answer someone by name, or write a starter in it.
export interface ComposerHandle {
  answer: (name: string) => void
  write: (said: string) => void
}

export function ConversationComposer({
  page,
  live,
  handle,
}: {
  page: ConversationData
  live: RoomLive
  handle: Ref<ComposerHandle>
}) {
  const [text, setText] = useState("")
  const box = useRef<HTMLTextAreaElement>(null)
  useImperativeHandle(handle, () => ({
    answer: (name) => {
      setText((now) => `@${name} ${now}`)
      box.current?.focus()
    },
    write: (said) => {
      setText(said)
      box.current?.focus()
    },
  }))
  const navigate = useNavigate()
  const [attached, setAttached] = useState<File[]>([])
  const [unsent, setUnsent] = useState("")
  const [sending, setSending] = useState(false)
  const room = page.Instance.Room
  // Sending while a reply is written takes the message up now: the reply is stopped, and the agent keeps what it did.
  const now = !room && page.Waiting
  const refresh = useRefreshAll()
  const sent = () => {
    setText("")
    setAttached([])
    setUnsent("")
  }
  const send = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (sending || (!text.trim() && attached.length === 0)) return
    setSending(true)
    try {
      if (room && live.online && attached.length === 0) {
        await live.say(text.trim())
        sent()
      } else {
        const landed = await sendForm(event.currentTarget, attached)
        sent()
        refresh()
        if (landed && landed.split("#")[0] !== page.Path) navigate(landed)
      }
    } catch (err) {
      // Not heard: the words stay in the box, and sending again sends them under the same id.
      setUnsent(
        `That was not sent. ${err instanceof Error ? err.message : "The connection dropped; press send to try again."}`
      )
    } finally {
      setSending(false)
    }
  }
  const shownUnsent = unsent || page.Unsent
  if (page.Settings.Archived) return <Archived page={page} />
  return (
    <form
      method="post"
      action={withBase(`${page.Path}/say`)}
      encType={room ? "multipart/form-data" : undefined}
      onSubmit={send}
      className="relative mx-auto w-full max-w-208"
    >
      {now && <input type="hidden" name="now" value="1" />}
      <p
        id="unsent"
        role={shownUnsent ? "alert" : undefined}
        hidden={!shownUnsent}
        className="m-0 mb-2 rounded-md bg-bad-wash px-4 py-2 text-sm text-bad"
      >
        {shownUnsent}
      </p>
      <Composer
        label={room ? "Message the room" : `Message ${page.Agent.Name}`}
        placeholder={room ? page.Hint : `Message ${page.Agent.Name}`}
        describedBy="unsent"
        autoFocus={!room}
        boxRef={box}
        text={text}
        canSend={!sending && (text.trim() !== "" || attached.length > 0)}
        sendLabel={now ? "Send now, stopping the reply" : "Send"}
        onWrite={(next) => {
          setText(next)
          if (next.trim()) live.typed()
        }}
        field={
          room ? (
            <>
              {attached.length > 0 && (
                <Attached
                  files={attached}
                  onRemove={(file) =>
                    setAttached(attached.filter((each) => each !== file))
                  }
                />
              )}
              <MentionBox
                box={box}
                everyone={page.Mentions ?? []}
                label="Message the room"
                placeholder={page.Hint}
                text={text}
                onWrite={(next) => {
                  setText(next)
                  if (next.trim()) live.typed()
                }}
              />
            </>
          ) : undefined
        }
        tools={
          room ? (
            <AttachButton
              onChoose={(files) => {
                const next = attach(
                  attached,
                  files,
                  page.AttachMost,
                  page.AttachBytes
                )
                setAttached(next.kept)
                setUnsent(next.refused)
              }}
            />
          ) : !page.Several ? (
            <span className="text-xs text-muted-foreground">
              {page.Agent.Brain}
            </span>
          ) : undefined
        }
        send={
          !room &&
          page.Waiting &&
          text.trim() === "" &&
          attached.length === 0 ? (
            <StopReply page={page} onStopped={refresh} />
          ) : undefined
        }
        actions={
          <>
            {!room && page.Several && (
              <ModelChoice page={page} onChanged={refresh} />
            )}
          </>
        }
      />
    </form>
  )
}

function Attached({
  files,
  onRemove,
}: {
  files: File[]
  onRemove: (file: File) => void
}) {
  return (
    <ul
      aria-label="Files attached to this message"
      className="m-0 flex list-none flex-wrap gap-1.5 p-0"
    >
      {files.map((file) => (
        <li
          key={`${file.name}-${file.size}-${file.lastModified}`}
          className="inline-flex h-7 max-w-full items-center gap-1.5 rounded-md border bg-muted pr-1 pl-2.5 text-xs"
        >
          <span className="truncate">{file.name}</span>
          <span className="flex-none text-subtle">{sizeWords(file.size)}</span>
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            aria-label={`Remove ${file.name}`}
            onClick={() => onRemove(file)}
          >
            <XIcon />
          </Button>
        </li>
      ))}
    </ul>
  )
}

function AttachButton({ onChoose }: { onChoose: (files: File[]) => void }) {
  return (
    <label
      className="grid size-8 flex-none cursor-pointer place-items-center rounded-full text-muted-foreground hover:bg-hover hover:text-foreground has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring [&>svg]:size-4.5"
      title="Attach files"
    >
      <PaperclipIcon aria-hidden="true" />
      <span className="sr-only">Attach files</span>
      <input
        className="sr-only"
        type="file"
        multiple
        onChange={(event) => {
          onChoose([...(event.currentTarget.files ?? [])])
          event.currentTarget.value = ""
        }}
      />
    </label>
  )
}

// ModelChoice changes the model the conversation's agent uses from its next turn.
function ModelChoice({
  page,
  onChanged,
}: {
  page: ConversationData
  onChanged: () => void
}) {
  return (
    <ChoiceSelect
      label="Model"
      choices={page.Models ?? []}
      value={page.Instance.Model}
      onChoose={(model) => {
        const form = document.createElement("form")
        form.method = "post"
        form.action = withBase(
          `/instances/${encodeURIComponent(page.Instance.ID)}/model`
        )
        for (const [name, value] of [
          ["model", model],
          ["back", page.Path],
        ]) {
          const field = document.createElement("input")
          field.name = name
          field.value = value
          form.append(field)
        }
        void sendForm(form).then(onChanged, onChanged)
      }}
    />
  )
}

// StopReply stops the reply being written. It stands where Send does while the box is empty.
function StopReply({
  page,
  onStopped,
}: {
  page: ConversationData
  onStopped: () => void
}) {
  return (
    <Button
      type="button"
      size="icon-sm"
      className="rounded-full"
      aria-label="Stop"
      title="Stop"
      onClick={() => {
        const form = document.createElement("form")
        form.method = "post"
        form.action = withBase(`/jobs/${encodeURIComponent(page.Latest)}/stop`)
        const back = document.createElement("input")
        back.name = "back"
        back.value = page.Path
        form.append(back)
        void sendForm(form).then(onStopped, onStopped)
      }}
    >
      <SquareIcon className="fill-current" />
    </Button>
  )
}

// Archived says nobody can say anything in the room until an admin brings it back, and offers that to one who can.
function Archived({ page }: { page: ConversationData }) {
  return (
    <div
      data-testid="archived"
      className="mx-auto flex w-full max-w-208 items-center gap-3 rounded-md border bg-sunken py-3 pr-3 pl-5 text-sm text-muted-foreground max-nav:flex-wrap"
    >
      <span className="min-w-0 flex-1">
        This room is archived. Nobody can say anything in it until an admin
        brings it back.
      </span>
      {page.Settings.Runs && (
        <form
          method="post"
          action={withBase(
            `/rooms/${encodeURIComponent(page.Instance.ID)}/archive`
          )}
          className="m-0"
        >
          <input type="hidden" name="archive" value="off" />
          <Button
            type="submit"
            variant="outline"
            size="sm"
            data-testid="unarchive"
          >
            Unarchive
          </Button>
        </form>
      )}
    </div>
  )
}
