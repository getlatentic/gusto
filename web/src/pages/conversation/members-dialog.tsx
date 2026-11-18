import { useRef, useState } from "react"
import { flushSync } from "react-dom"
import { BotIcon, PlusIcon, UserIcon } from "lucide-react"

import { AgentMark } from "@/components/controls"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { initial } from "@/lib/format"
import { useConversation } from "@/pages/conversation/conversation-context"
import type { RoomAgent, RoomPerson } from "@/pages/conversation/types"
import { withBase } from "@/lib/base"

const row = "flex min-h-10 items-center gap-3 px-1"
const face =
  "grid size-7 flex-none place-items-center overflow-hidden rounded-full bg-sunken text-xs font-medium text-muted-foreground"

// MembersDialog is who is in the room and which agents: an admin changes roles and removes or accepts members; anyone leaves.
export function MembersDialog({
  open,
  onClose,
  onAdd,
}: {
  open: boolean
  onClose: () => void
  onAdd: () => void
}) {
  const { page } = useConversation()
  const members = page.Members
  if (!members) return null
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent
        data-testid="members-dialog"
        className="grid gap-1 sm:max-w-108"
      >
        <DialogHeader className="mb-3">
          <DialogTitle>Room members</DialogTitle>
        </DialogHeader>
        {page.Refused && (
          <p
            role="alert"
            className="m-0 mb-2 rounded-md bg-bad-wash px-3 py-2 text-sm text-bad"
          >
            {page.Refused}
          </p>
        )}
        <Button
          variant="ghost"
          className="justify-start px-1"
          data-testid="add-member"
          onClick={onAdd}
        >
          <PlusIcon /> {members.Runs ? "Add member" : "Propose an agent"}
        </Button>
        <ul aria-label="People" className="m-0 grid list-none p-0">
          {(members.People ?? []).map((person) => (
            <li key={person.Who}>
              <Person person={person} runs={members.Runs} />
            </li>
          ))}
        </ul>
        <ul aria-label="Agents" className="m-0 grid list-none p-0">
          {(members.Agents ?? []).map((agent) => (
            <li key={agent.ID}>
              <Agent agent={agent} runs={members.Runs} />
            </li>
          ))}
        </ul>
      </DialogContent>
    </Dialog>
  )
}

// ChoiceForm is a form that sends itself as soon as its one choice is made.
function ChoiceForm({
  action,
  name,
  value,
  label,
  choices,
  hidden,
  testId,
}: {
  action: string
  name: string
  value: string
  label: string
  choices: { value: string; label: string }[]
  hidden?: Record<string, string>
  testId?: string
}) {
  const { submit } = useConversation()
  const form = useRef<HTMLFormElement>(null)
  const [chosen, setChosen] = useState(value)
  return (
    <form
      ref={form}
      method="post"
      action={withBase(action)}
      onSubmit={submit}
      className="m-0"
    >
      {Object.entries(hidden ?? {}).map(([key, held]) => (
        <input key={key} type="hidden" name={key} value={held} />
      ))}
      <Select
        name={name}
        items={choices}
        value={chosen}
        onValueChange={(next) => {
          // The form's field holds the choice once it is drawn with it, and only then is the form sent.
          flushSync(() => setChosen(String(next ?? "")))
          form.current?.requestSubmit()
        }}
      >
        <SelectTrigger
          size="sm"
          aria-label={label}
          data-testid={testId}
          className="border-0 shadow-none"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {choices.map((choice) => (
            <SelectItem
              key={choice.value}
              value={choice.value}
              data-testid="choice-option"
              data-value={choice.value}
            >
              {choice.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </form>
  )
}

function Person({ person, runs }: { person: RoomPerson; runs: boolean }) {
  const { page, submit } = useConversation()
  const room = `/rooms/${encodeURIComponent(page.Instance.ID)}`
  return (
    <div className={row} data-testid="room-person" data-who={person.Who}>
      <span className={face} aria-hidden="true">
        {initial(person.Name) || <UserIcon className="size-3.5" />}
      </span>
      <span className="min-w-0 flex-1 truncate text-sm" title={person.Who}>
        {person.Name}
        {person.Me && <span className="text-subtle"> you</span>}
      </span>
      {runs ? (
        <ChoiceForm
          action={`${room}/members/role`}
          name="role"
          value={person.Role === "admin" ? "admin" : "member"}
          label={`Role of ${person.Name}`}
          testId="room-role"
          hidden={{ who: person.Who }}
          choices={[
            { value: "admin", label: "Admin" },
            { value: "member", label: "Member" },
            {
              value: "remove",
              label: person.Me ? "Leave the room" : "Remove from room",
            },
          ]}
        />
      ) : (
        <>
          <span
            className="text-sm text-muted-foreground"
            data-testid="room-role"
          >
            {person.Role === "admin" ? "Admin" : "Member"}
          </span>
          {person.Me && (
            <form
              method="post"
              action={withBase(`${room}/members/remove`)}
              onSubmit={submit}
              className="m-0"
            >
              <input type="hidden" name="who" value={person.Who} />
              <Button
                type="submit"
                variant="link"
                size="sm"
                className="text-subtle"
              >
                Leave
              </Button>
            </form>
          )}
        </>
      )}
    </div>
  )
}

function Agent({ agent, runs }: { agent: RoomAgent; runs: boolean }) {
  const { page, submit } = useConversation()
  const at = `/rooms/${encodeURIComponent(page.Instance.ID)}/agents/${encodeURIComponent(agent.ID)}`
  const details = [agent.Engine, agent.Brain, agent.Allowance]
    .filter(Boolean)
    .join(" · ")
  return (
    <>
      <div className={row}>
        <span className={face} aria-hidden="true">
          {agent.Mark ? (
            <AgentMark
              mark={agent.Mark}
              markDark={agent.MarkDark}
              name={agent.Engine}
            />
          ) : (
            initial(agent.Name)
          )}
        </span>
        <span className="flex min-w-0 flex-1 items-center gap-1.5 text-sm">
          <span className="flex-none">{agent.Name}</span>
          <span className="truncate text-subtle" title={details}>
            {agent.Job || agent.Engine}
          </span>
          {agent.Owner && (
            <span className="flex-none text-subtle">· {agent.Owner}</span>
          )}
          <BotIcon
            aria-label="Agent"
            className="size-3.5 flex-none text-subtle"
          />
        </span>
        {agent.Pending && runs ? (
          <>
            <form
              method="post"
              action={withBase(`${at}/accept`)}
              onSubmit={submit}
              className="m-0"
            >
              <Button type="submit" size="sm">
                Accept
              </Button>
            </form>
            <form
              method="post"
              action={withBase(`${at}/remove`)}
              onSubmit={submit}
              className="m-0"
            >
              <Button
                type="submit"
                variant="link"
                size="sm"
                className="text-subtle"
              >
                Decline
              </Button>
            </form>
          </>
        ) : runs || agent.Mine ? (
          <ChoiceForm
            action={`${at}/remove`}
            name="keep"
            value=""
            label={agent.Name}
            choices={[
              {
                value: "",
                label: agent.Pending ? "Waiting for an admin" : "Agent",
              },
              {
                value: "remove",
                label: agent.Pending ? "Withdraw" : "Remove from room",
              },
            ]}
          />
        ) : (
          <span className="text-sm text-muted-foreground">
            {agent.Pending ? "Waiting for an admin" : "Agent"}
          </span>
        )}
      </div>
      {(runs || agent.Mine) && (
        <form
          method="post"
          action={withBase(`${at}/allowance`)}
          onSubmit={submit}
          className="m-0 flex items-center gap-2 pb-2 pl-11"
        >
          <label className="flex items-center gap-2 text-xs text-subtle">
            Allowance a month
            <Input
              name="allowance"
              inputMode="decimal"
              placeholder="no limit"
              autoComplete="off"
              defaultValue={agent.AllowanceField}
              aria-label={`${agent.Name}'s allowance a month`}
              className="h-7 w-24"
            />
          </label>
          <Button type="submit" variant="outline" size="sm">
            Save
          </Button>
        </form>
      )}
    </>
  )
}
