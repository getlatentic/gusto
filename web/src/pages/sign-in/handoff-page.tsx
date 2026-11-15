import { useEffect } from "react"

import { buttonVariants } from "@/components/ui/button"
import { leaveFor } from "@/lib/leave"
import { api } from "@/store/api"
import { cn } from "@/lib/utils"
import { PublicPanel } from "@/pages/sign-in/public-panel"

const ticketPath = "/app/api/auth/desktop/handoff"

// HandoffPage sends the browser a person signed in with back to the desktop app, with a ticket the app trades for its own session.
// A ticket is made once, so it is read once: not again when the window comes back into view.
export function HandoffPage() {
  const read = api.endpoints.page.useQuery(ticketPath, {
    refetchOnFocus: false,
    refetchOnReconnect: false,
  })
  const link = (read.data as { Link: string } | undefined)?.Link
  // The app is outside this page: opening its link is what the page is for, as soon as there is one.
  useEffect(() => {
    if (link) leaveFor(link)
  }, [link])
  return (
    <PublicPanel className="max-w-sm text-center">
      <h1 className="m-0 text-lg">You are signed in</h1>
      <p className="m-0 text-muted-foreground">
        Return to gusto to carry on. You can close this tab.
      </p>
      {read.isError ? (
        <p role="alert" className="m-0 text-sm text-bad">
          This sign-in was already handed to the app. Start it again from the
          app.
        </p>
      ) : (
        <a
          className={cn(buttonVariants(), "h-11")}
          href={link}
          aria-disabled={!link}
          data-testid="open-app"
        >
          Open gusto
        </a>
      )}
    </PublicPanel>
  )
}
