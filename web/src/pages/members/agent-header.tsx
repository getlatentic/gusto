import { useRef, useState } from "react"
import { BotIcon, MessageSquareIcon, Trash2Icon } from "lucide-react"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { AgentFace } from "@/pages/members/faces"
import type { AgentPage } from "@/pages/members/types"
import { useMembersForm } from "@/pages/members/use-members-form"

// AgentHeader is who the agent is, and what the person looking may do with it: message it in their own room with it, and, when
// it is theirs, remove it, which is asked first.
export function AgentHeader({ agent }: { agent: AgentPage }) {
  const form = useMembersForm()
  const at = `/members/agents/${encodeURIComponent(agent.ID)}`
  return (
    <header className="grid gap-3">
      <div className="flex items-center gap-4">
        <AgentFace mark={agent.Mark} name={agent.Name} size="lg" />
        <div className="grid min-w-0 flex-1">
          <h2 className="m-0 flex items-center gap-2 text-lg font-semibold">
            <span className="truncate">{agent.Name}</span>
            <BotIcon
              role="img"
              aria-label="Agent"
              className="size-4 flex-none text-subtle"
            />
          </h2>
          <p className="m-0 truncate text-sm text-muted-foreground">
            {[agent.Engine, agent.Brain].filter(Boolean).join(" · ")}
          </p>
        </div>
        {agent.Usable && (
          <form
            method="post"
            action={`${at}/message`}
            onSubmit={form.submit}
            className="m-0"
            data-testid="message-agent-form"
          >
            <Button type="submit" variant="outline" disabled={form.sending}>
              <MessageSquareIcon /> Message
            </Button>
          </form>
        )}
        {agent.Mine && <RemoveAgent agent={agent} at={at} send={form.send} />}
      </div>
      {form.failed && (
        <p
          role="alert"
          className="m-0 rounded-md bg-bad-wash px-4 py-3 text-sm text-bad"
        >
          {form.failed}
        </p>
      )}
    </header>
  )
}

function RemoveAgent({
  agent,
  at,
  send,
}: {
  agent: AgentPage
  at: string
  send: (form: HTMLFormElement) => Promise<void>
}) {
  const remove = useRef<HTMLFormElement>(null)
  const [asking, setAsking] = useState(false)
  return (
    <>
      <form
        ref={remove}
        method="post"
        action={`${at}/remove`}
        className="m-0"
        data-testid="remove-agent-form"
        onSubmit={(event) => {
          event.preventDefault()
          setAsking(true)
        }}
      >
        <Button
          type="submit"
          variant="ghost"
          size="icon"
          aria-label={`Remove ${agent.Name}`}
          title={`Remove ${agent.Name}`}
          className="text-muted-foreground hover:bg-bad-wash hover:text-bad"
        >
          <Trash2Icon />
        </Button>
      </form>
      <AlertDialog open={asking} onOpenChange={setAsking}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove {agent.Name}?</AlertDialogTitle>
            <AlertDialogDescription>
              It leaves every room it is in, and what it said there stays.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              data-testid="confirm-remove"
              variant="destructive"
              onClick={() => {
                setAsking(false)
                if (remove.current) void send(remove.current)
              }}
            >
              Remove
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
