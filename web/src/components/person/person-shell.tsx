import { useMemo, useState } from "react"
import { Outlet, useLocation } from "react-router"
import { PlusIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { NavigationContext } from "@/components/navigation"
import { SkipLink } from "@/components/skip-link"
import { EmailReminder } from "@/components/person/email-reminder"
import { usePanelWidth } from "@/components/person/panel-resize"
import { PlacesRail } from "@/components/person/places-rail"
import { showsConversations } from "@/components/person/places"
import { SideList } from "@/components/person/side-list"
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet"
import type { PersonShell as Shell } from "@/domain/side-list"
import { useShell } from "@/lib/shell"

// PersonShell is what a person's own pages sit in: the rail of their places, and beside a conversation the list of them all.
// On a narrow window both fold into a sheet the page's menu button opens, so a page has one bar at its top.
export function PersonShell() {
  const shell = useShell().data
  const here = useLocation().pathname
  const [open, setOpen] = useState(false)
  const navigation = useMemo(() => ({ open: () => setOpen(true) }), [])
  const { width, divider } = usePanelWidth()
  const conversations = showsConversations(here)
  return (
    <NavigationContext.Provider value={navigation}>
      <SkipLink />
      <div className="grid h-full grid-cols-[auto_minmax(0,1fr)] max-nav:grid-cols-[minmax(0,1fr)]">
        <aside
          aria-label="Menu"
          className="relative flex min-h-0 max-nav:hidden"
          style={{ "--panel-w": `${width}rem` } as React.CSSProperties}
        >
          <PlacesRail shell={shell} here={here} />
          {conversations && (
            <>
              <SidePanel shell={shell} here={here} />
              <div
                role="separator"
                aria-orientation="vertical"
                aria-label="Resize the conversations"
                aria-valuenow={width}
                tabIndex={0}
                className="absolute inset-y-0 -right-1 z-10 w-2 cursor-col-resize touch-none after:absolute after:inset-y-0 after:left-0.75 after:w-0.5 after:content-[''] hover:after:bg-input focus-visible:after:bg-ring"
                {...divider}
              />
            </>
          )}
        </aside>
        <Sheet open={open} onOpenChange={setOpen}>
          <SheetContent
            side="left"
            className="flex w-auto max-w-[92vw] flex-row gap-0 p-0"
          >
            <SheetTitle className="sr-only">Menu</SheetTitle>
            <PlacesRail shell={shell} here={here} onGo={() => setOpen(false)} />
            <SidePanel shell={shell} here={here} onGo={() => setOpen(false)} />
          </SheetContent>
        </Sheet>
        <main
          id="main"
          tabIndex={-1}
          className="flex min-h-0 min-w-0 flex-col overflow-y-auto"
        >
          <Outlet />
        </main>
      </div>
    </NavigationContext.Provider>
  )
}

// SidePanel is the list of conversations, under the organization's name and the way to start a new one.
function SidePanel({
  shell,
  here,
  onGo,
}: {
  shell?: Shell
  here: string
  onGo?: () => void
}) {
  return (
    <section
      aria-label="Your conversations"
      className="flex min-h-0 w-(--panel-w,20rem) min-w-0 flex-none flex-col gap-4 bg-side p-3 max-nav:w-72"
    >
      <header className="flex min-h-10 items-center gap-2 pr-1 pl-3">
        <span className="min-w-0 flex-1 truncate font-heading text-base font-semibold">
          {shell?.Org?.Name ?? "gusto"}
        </span>
        <AppLink
          href="/chat"
          aria-label="New chat"
          title="New chat"
          aria-current={here === "/chat" ? "page" : undefined}
          className="grid size-8 flex-none place-items-center rounded-md text-muted-foreground hover:bg-hover hover:text-foreground aria-[current=page]:bg-hover aria-[current=page]:text-foreground [&>svg]:size-4"
          onClick={onGo}
        >
          <PlusIcon />
        </AppLink>
      </header>
      {shell && <SideList rail={shell.Rail} here={here} onGo={onGo} />}
      {shell?.Org && (
        <EmailReminder
          from={shell.Org.AccountFrom}
          send={shell.Org.ConfirmAgain}
        />
      )}
    </section>
  )
}
