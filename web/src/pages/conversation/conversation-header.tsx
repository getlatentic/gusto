import { FileTextIcon, ShieldCheckIcon, UsersIcon } from "lucide-react"

import { Button, buttonVariants } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import type { Present } from "@/lib/room-socket"
import { useConversation } from "@/pages/conversation/conversation-context"
import type { RoomDialog } from "@/pages/conversation/room-dialog"
import { RoomMenu } from "@/pages/conversation/room-menu"

const labelled = "max-sm:[&>span]:sr-only"

// ConversationActions are what the page bar of a conversation offers at its right: who is here, where the agent runs, the app
// it made, its files, and in a room its members and everything else.
export function ConversationActions({
  here,
  onFiles,
  onDialog,
}: {
  here: Present[]
  onFiles: () => void
  onDialog: (dialog: RoomDialog) => void
}) {
  const { page } = useConversation()
  const room = page.Instance.Room
  return (
    <div className="flex flex-none items-center gap-1">
      {here.length > 0 && (
        <ul
          aria-label="In this room now"
          className="m-0 mr-2 flex list-none items-center -space-x-1.5 p-0"
        >
          {here.map((person) => (
            <li
              key={person.user}
              title={`${person.name} is here`}
              className="grid size-6 place-items-center rounded-full bg-sunken text-[0.625rem] font-medium text-muted-foreground ring-2 ring-background"
            >
              {person.short}
            </li>
          ))}
        </ul>
      )}
      <Sandbox />
      {(page.Apps ?? []).map((app) => (
        <a
          key={app.Link}
          className={buttonVariants({ variant: "ghost", size: "sm" })}
          href={app.Link}
          target="_blank"
          rel="noopener"
        >
          Open the app
        </a>
      ))}
      {page.Starting && (
        <span className="text-xs text-muted-foreground">
          Starting the preview…
        </span>
      )}
      {room ? (
        <Button
          variant="ghost"
          size="sm"
          className={labelled}
          onClick={onFiles}
        >
          <FileTextIcon /> <span>Files</span>
        </Button>
      ) : (
        <a
          className={`${buttonVariants({ variant: "ghost", size: "sm" })} ${labelled}`}
          href={`/instances/${encodeURIComponent(page.Instance.ID)}/files?from=${encodeURIComponent(page.Path)}`}
        >
          <FileTextIcon /> <span>Files</span>
        </a>
      )}
      {room && (
        <>
          <Button
            variant="ghost"
            size="sm"
            className="tabular-nums"
            aria-label={`Room members, ${page.Headcount}`}
            title="Room members"
            onClick={() => onDialog("members")}
          >
            <UsersIcon /> {page.Headcount}
          </Button>
          <RoomMenu onDialog={onDialog} />
        </>
      )}
    </div>
  )
}

// Sandbox says where the agent runs, beside Files and labelled like it: in a sandbox of its own, with the model's key kept
// from it, and for a conversation the folder it works in.
function Sandbox() {
  const { page } = useConversation()
  return (
    <Popover>
      <PopoverTrigger
        render={<Button variant="ghost" size="sm" className={labelled} />}
      >
        <ShieldCheckIcon /> <span>Sandboxed</span>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 text-sm">
        <p className="m-0">
          Runs in its own sandbox. The model&apos;s key never reaches the agent.
          {page.Instance.Sensitivity === "restricted" &&
            " It stays in your network."}
        </p>
        {!page.Instance.Room && page.Thread.Dir && (
          <p className="m-0 mt-2 text-muted-foreground">
            Works in the folder{" "}
            <span className="font-medium text-foreground">
              {page.Thread.Dir}
            </span>
            .
          </p>
        )}
      </PopoverContent>
    </Popover>
  )
}
