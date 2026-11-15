import { useState } from "react"
import { CheckIcon, PlusIcon, SearchIcon } from "lucide-react"

import { AgentMark } from "@/components/controls"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { initial } from "@/lib/format"
import { cn } from "@/lib/utils"
import { useConversation } from "@/pages/conversation/conversation-context"

// AddDialog adds agents and people to a room. A member who does not run the room proposes an agent of theirs, which joins when
// an admin accepts it.
export function AddDialog({
  open,
  onClose,
}: {
  open: boolean
  onClose: () => void
}) {
  const { page, submit } = useConversation()
  const [filter, setFilter] = useState("")
  const members = page.Members
  if (!members) return null
  const runs = members.Runs
  const addable = members.Addable ?? []
  const shown = addable.filter((agent) =>
    agent.Name.toLowerCase().includes(filter.trim().toLowerCase())
  )
  const none =
    members.RoomAgents === 0
      ? "You have no agents yet. Make one with New agent, then add it here."
      : runs
        ? "Every agent you work with is in this room already."
        : "Every agent of yours is in this room already."
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-120">
        <form
          method="post"
          action={`/rooms/${encodeURIComponent(page.Instance.ID)}/members/add`}
          onSubmit={submit}
          className="grid gap-4"
        >
          <DialogHeader>
            <DialogTitle>
              {runs ? "Add members" : "Propose an agent"}
            </DialogTitle>
            <DialogDescription>
              {runs
                ? "Pick the agents you work with, or add people by their email address."
                : "Pick an agent of yours. It joins when an admin of the room accepts it."}
            </DialogDescription>
          </DialogHeader>
          {page.AddRefused && (
            <p
              role="alert"
              className="m-0 rounded-md bg-bad-wash px-3 py-2 text-sm text-bad"
            >
              {page.AddRefused}
            </p>
          )}
          {addable.length > 0 ? (
            <>
              <div className="relative">
                <SearchIcon
                  aria-hidden="true"
                  className="absolute top-2.5 left-3 size-4 text-muted-foreground"
                />
                <Input
                  type="search"
                  placeholder="Search agents"
                  aria-label="Search agents"
                  autoComplete="off"
                  className="pl-9"
                  value={filter}
                  onChange={(event) => setFilter(event.target.value)}
                />
              </div>
              <ul
                aria-label="Agents"
                className="m-0 grid max-h-64 list-none gap-1.5 overflow-y-auto p-0"
              >
                {addable.map((agent) => (
                  <li key={agent.ID} hidden={!shown.includes(agent)}>
                    <label className="flex h-11 cursor-pointer items-center gap-3 rounded-lg border px-3 text-sm hover:border-input has-[:checked]:border-primary has-[:checked]:bg-primary/5">
                      <input
                        type="checkbox"
                        name="agent"
                        value={agent.ID}
                        className="peer sr-only"
                      />
                      <span
                        className="grid size-6 flex-none place-items-center overflow-hidden rounded-full bg-sunken text-xs"
                        aria-hidden="true"
                      >
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
                      <span className="flex-none font-medium">
                        {agent.Name}
                      </span>
                      <span className="min-w-0 truncate text-subtle">
                        {agent.Specialty || agent.Engine}
                      </span>
                      <CheckIcon
                        aria-hidden="true"
                        className="ml-auto hidden size-4 flex-none text-primary peer-checked:block"
                      />
                    </label>
                  </li>
                ))}
              </ul>
              <div className="grid gap-2">
                <Label htmlFor="room-allowance">
                  Allowance{" "}
                  <span className="font-normal text-muted-foreground">
                    dollars each agent may spend in this room a month; empty for
                    no limit
                  </span>
                </Label>
                <Input
                  id="room-allowance"
                  name="allowance"
                  inputMode="decimal"
                  placeholder="no limit"
                  autoComplete="off"
                />
              </div>
            </>
          ) : (
            <p className="m-0 text-sm text-muted-foreground">{none}</p>
          )}
          {runs && (
            <div className="grid gap-2">
              <Label htmlFor="room-people">
                People{" "}
                <span className="font-normal text-muted-foreground">
                  email addresses, separated by commas
                </span>
              </Label>
              <Input
                id="room-people"
                name="who"
                placeholder="name@example.com"
                autoComplete="off"
              />
            </div>
          )}
          <DialogFooter className="items-center">
            <a
              className={cn(
                buttonVariants({ variant: "link" }),
                "mr-auto px-0"
              )}
              href="/members?new=1"
            >
              <PlusIcon /> New agent
            </a>
            <DialogClose render={<Button variant="outline" type="button" />}>
              Cancel
            </DialogClose>
            <Button type="submit">{runs ? "Add" : "Propose"}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
