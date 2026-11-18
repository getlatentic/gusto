import { Button } from "@/components/ui/button"
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
import type { Invite } from "@/pages/members/types"
import { withBase } from "@/lib/base"

// InviteDialog invites a person to the organization. The form goes to gusto cloud, which keeps who is in it, as the browser sends
// it: gusto cloud asks again whether the person may invite, and sends them back to this page saying the invitation went.
export function InviteDialog({
  invite,
  open,
  onClose,
}: {
  invite: Invite
  open: boolean
  onClose: () => void
}) {
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-108" data-testid="invite-dialog">
        <form
          method="post"
          action={withBase(invite.Action)}
          className="grid gap-5"
          data-testid="invite-form"
        >
          <DialogHeader>
            <DialogTitle>Invite a person</DialogTitle>
            <DialogDescription>
              They are mailed a link that works once, for seven days. They join
              this team when they open it.
            </DialogDescription>
          </DialogHeader>
          <input type="hidden" name="team" value={invite.Team} />
          <input type="hidden" name="back" value={invite.Back} />
          <div className="grid gap-2">
            <Label htmlFor="invite-email">Email</Label>
            <Input
              id="invite-email"
              type="email"
              name="email"
              placeholder="ada@company.com"
              autoComplete="off"
              required
            />
          </div>
          <DialogFooter>
            <DialogClose render={<Button variant="outline" type="button" />}>
              Cancel
            </DialogClose>
            <Button type="submit">Send the invitation</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
