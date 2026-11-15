import { useState } from "react"
import {
  ArchiveIcon,
  BellIcon,
  BellOffIcon,
  EllipsisIcon,
  FileTextIcon,
  LinkIcon,
  LogOutIcon,
  Trash2Icon,
  UsersIcon,
} from "lucide-react"

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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { useConversation } from "@/pages/conversation/conversation-context"
import type { RoomDialog } from "@/pages/conversation/room-dialog"

interface Asking {
  title: string
  words: string
  doing: string
  action: string
  fields: Record<string, string>
}

// RoomMenu is what else a room offers: its link, its guidelines and members, muting it, and leaving, archiving or deleting it.
// What cannot be undone easily is asked first.
export function RoomMenu({
  onDialog,
}: {
  onDialog: (dialog: RoomDialog) => void
}) {
  const { page, post } = useConversation()
  const [asking, setAsking] = useState<Asking>()
  const [copied, setCopied] = useState(false)
  const room = `/rooms/${encodeURIComponent(page.Instance.ID)}`
  const title = page.Instance.Title
  const settings = page.Settings
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="More for this room"
              data-testid="room-menu"
              title="More"
            />
          }
        >
          <EllipsisIcon />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-64">
          <DropdownMenuItem
            closeOnClick={false}
            onClick={() => {
              void navigator.clipboard?.writeText(
                location.origin + location.pathname
              )
              setCopied(true)
              window.setTimeout(() => setCopied(false), 1_500)
            }}
          >
            <LinkIcon /> {copied ? "Link copied" : "Copy link"}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => onDialog("guidelines")}>
            <FileTextIcon /> Room guidelines
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => onDialog("members")}>
            <UsersIcon /> Room members
          </DropdownMenuItem>
          {settings.In && (
            <DropdownMenuItem
              data-testid="room-mute"
              onClick={() =>
                void post(`${room}/mute`, {
                  mute: settings.Muted ? "off" : "on",
                })
              }
            >
              {settings.Muted ? <BellIcon /> : <BellOffIcon />}{" "}
              {settings.Muted ? "Unmute room" : "Mute room"}
            </DropdownMenuItem>
          )}
          {settings.Runs && !page.Instance.Direct && (
            <DropdownMenuItem
              data-testid="room-archive"
              onClick={() =>
                settings.Archived
                  ? void post(`${room}/archive`, { archive: "off" })
                  : setAsking({
                      title: `Archive ${title}?`,
                      words:
                        "It leaves everybody's list and nobody can say anything in it until an admin brings it back.",
                      doing: "Archive room",
                      action: `${room}/archive`,
                      fields: { archive: "on" },
                    })
              }
            >
              <ArchiveIcon />{" "}
              {settings.Archived ? "Unarchive room" : "Archive room"}
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={() =>
              setAsking({
                title: `Leave ${title}?`,
                words: "You can be added again by its admin.",
                doing: "Leave room",
                action: `${room}/members/remove`,
                fields: { who: page.Me },
              })
            }
          >
            <LogOutIcon /> Leave room
          </DropdownMenuItem>
          {page.Owns && (
            <DropdownMenuItem
              variant="destructive"
              onClick={() =>
                setAsking({
                  title: `Delete ${title}?`,
                  words:
                    "It goes for everybody in it, with its conversation and its files. This cannot be undone.",
                  doing: "Delete room",
                  action: `/instances/${encodeURIComponent(page.Instance.ID)}/remove`,
                  fields: {},
                })
              }
            >
              <Trash2Icon /> Delete room
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      <AlertDialog
        open={Boolean(asking)}
        onOpenChange={(open) => !open && setAsking(undefined)}
      >
        <AlertDialogContent data-testid="room-confirm">
          <AlertDialogHeader>
            <AlertDialogTitle>{asking?.title}</AlertDialogTitle>
            <AlertDialogDescription>{asking?.words}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              data-testid="room-confirm-action"
              variant={
                asking?.doing === "Delete room" ? "destructive" : "default"
              }
              onClick={() => {
                if (asking) void post(asking.action, asking.fields)
                setAsking(undefined)
              }}
            >
              {asking?.doing}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
