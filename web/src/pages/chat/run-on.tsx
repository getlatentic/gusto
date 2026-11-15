import { ChevronDownIcon } from "lucide-react"

import { AgentIcon } from "@/components/agent-icon"
import { Button } from "@/components/ui/button"
import { runsOn, type Choices, type Offer } from "@/domain/chat-choices"

// RunOn says in one line what the chat will run on: the preset if there is one, the agent's mark, and the model. Pressing it
// shows the choices, so a person can type and send without choosing.
export function RunOn({
  offer,
  choices,
  open,
  onToggle,
}: {
  offer: Offer
  choices: Choices
  open: boolean
  onToggle: () => void
}) {
  const shown = runsOn(offer, choices)
  const words = `Runs on ${shown.words}. Change`
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="max-w-[min(22rem,100%)] min-w-0 rounded-full text-xs font-medium text-muted-foreground"
      aria-expanded={open}
      aria-controls="chat-choices"
      aria-label={words}
      title={words}
      onClick={onToggle}
    >
      {shown.agent?.icon && (
        <AgentIcon src={shown.agent.icon} className="size-4" />
      )}
      <span className="truncate">
        {[
          shown.preset,
          shown.agent?.icon ? "" : (shown.agent?.label ?? ""),
          shown.model,
        ]
          .filter(Boolean)
          .join(" · ")}
      </span>
      <ChevronDownIcon className="opacity-60" />
    </Button>
  )
}
