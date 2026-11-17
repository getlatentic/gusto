import { isRouteErrorResponse, useRouteError } from "react-router"

import { Button } from "@/components/ui/button"

// RouteError is what the app shows when a page could not be drawn: its code failed to load, as after gusto was updated while the
// page was open, or it broke while drawing. Reloading fetches the current code.
export function RouteError() {
  const error = useRouteError()
  const missing = isRouteErrorResponse(error) && error.status === 404
  return (
    <main
      className="grid min-h-dvh place-items-center p-8 text-center"
      data-testid="route-error"
    >
      <div className="grid max-w-sm justify-items-center gap-3">
        <h1 className="m-0 text-base font-semibold">
          {missing ? "There is nothing here." : "This page could not be shown."}
        </h1>
        {!missing && (
          <p className="m-0 text-sm text-muted-foreground">
            gusto may have been updated since the page was opened.
          </p>
        )}
        <Button onClick={() => window.location.reload()}>Reload</Button>
      </div>
    </main>
  )
}
