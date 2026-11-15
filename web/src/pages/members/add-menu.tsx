import { BotIcon, PlusIcon, UserIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

// AddMenu is the members' +: a menu of inviting a person and making an agent where somebody can be invited, and the new agent
// dialog by itself where nobody can.
export function AddMenu({
  invites,
  keeps,
  onInvite,
  onNewAgent,
}: {
  invites: boolean
  keeps: boolean
  onInvite: () => void
  onNewAgent: () => void
}) {
  if (!invites) {
    if (!keeps) return null
    return (
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="New agent"
        title="New agent"
        data-testid="new-agent"
        onClick={onNewAgent}
      >
        <PlusIcon />
      </Button>
    )
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Add a member"
            title="Add a member"
            data-testid="add-member"
          />
        }
      >
        <PlusIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuItem onClick={onInvite}>
          <UserIcon /> Invite a person
        </DropdownMenuItem>
        {keeps && (
          <DropdownMenuItem onClick={onNewAgent}>
            <BotIcon /> New agent
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
