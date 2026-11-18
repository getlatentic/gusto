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
import { Textarea } from "@/components/ui/textarea"
import { useConversation } from "@/pages/conversation/conversation-context"
import { withBase } from "@/lib/base"

// GuidelinesDialog is what every agent in the room is told at the start of each turn: everybody reads them, admins change them.
export function GuidelinesDialog({
  open,
  onClose,
}: {
  open: boolean
  onClose: () => void
}) {
  const { page, submit } = useConversation()
  const runs = page.Settings.Runs
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-120">
        <form
          method="post"
          action={withBase(
            `/rooms/${encodeURIComponent(page.Instance.ID)}/guidelines`
          )}
          onSubmit={submit}
          className="grid gap-4"
        >
          <DialogHeader>
            <DialogTitle>Room guidelines</DialogTitle>
            <DialogDescription>
              What every agent in this room is told at the start of each turn.
              Everybody in the room can read them
              {runs ? "; admins change them" : ""}.
            </DialogDescription>
          </DialogHeader>
          {page.GuidelinesRefused && (
            <p
              role="alert"
              className="m-0 rounded-md bg-bad-wash px-3 py-2 text-sm text-bad"
            >
              {page.GuidelinesRefused}
            </p>
          )}
          {runs ? (
            <>
              <Textarea
                name="guidelines"
                aria-label="Guidelines"
                maxLength={page.MaxGuidelines}
                defaultValue={page.Guidelines}
                placeholder="For example: answer in French, and ask before deleting files."
                className="min-h-32 resize-y leading-relaxed"
              />
              <DialogFooter className="items-center">
                <span className="mr-auto text-xs text-subtle">
                  Up to {page.MaxGuidelines} characters
                </span>
                <DialogClose
                  render={<Button variant="outline" type="button" />}
                >
                  Cancel
                </DialogClose>
                <Button type="submit">Save</Button>
              </DialogFooter>
            </>
          ) : page.Guidelines ? (
            <p className="m-0 text-sm leading-relaxed wrap-anywhere whitespace-pre-wrap">
              {page.Guidelines}
            </p>
          ) : (
            <p className="m-0 text-sm text-muted-foreground">
              This room has no guidelines.
            </p>
          )}
        </form>
      </DialogContent>
    </Dialog>
  )
}
