import { useMemo, useRef, useState } from "react"
import { Outlet, useLocation } from "react-router"
import { ArrowLeftIcon, LogOutIcon, SettingsIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { NavigationContext } from "@/components/navigation"
import { SkipLink } from "@/components/skip-link"
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet"
import { useData } from "@/lib/api"
import { usePageEnter } from "@/motion/use-page-enter"
import { useChildPath } from "@/router/use-child-path"
import type { ConsoleShell as Shell } from "@/lib/types"
import { cn } from "@/lib/utils"

// The console is the admin's own place: its sections down the side, one way back to the pages everybody uses, and no other
// link between the two.
export function ConsoleShell() {
  const shell = useData<Shell>("/app/api/console")
  const [open, setOpen] = useState(false)
  const main = useRef<HTMLElement>(null)
  usePageEnter(main, useChildPath("admin"))
  const navigation = useMemo(() => ({ open: () => setOpen(true) }), [])
  const sections = shell.state === "ready" ? shell.data : undefined
  return (
    <NavigationContext.Provider value={navigation}>
      <SkipLink />
      <div className="grid h-full grid-cols-[15.5rem_minmax(0,1fr)] max-nav:grid-cols-[minmax(0,1fr)]">
        <aside className="flex min-h-0 flex-col overflow-y-auto bg-sidebar px-3 py-4 max-nav:hidden">
          <ConsoleNav shell={sections} />
        </aside>
        <Sheet open={open} onOpenChange={setOpen}>
          <SheetContent side="left" className="w-64 bg-sidebar px-3 py-4">
            <SheetTitle className="sr-only">Admin</SheetTitle>
            <ConsoleNav shell={sections} onGo={() => setOpen(false)} />
          </SheetContent>
        </Sheet>
        <main
          ref={main}
          id="main"
          className="min-h-0 min-w-0 overflow-y-auto"
          tabIndex={-1}
        >
          <Outlet />
        </main>
      </div>
    </NavigationContext.Provider>
  )
}

function ConsoleNav({ shell, onGo }: { shell?: Shell; onGo?: () => void }) {
  const here = useLocation().pathname
  const item =
    "flex h-9 w-full min-w-0 items-center gap-2 rounded-md px-3 text-left text-sm text-sidebar-foreground no-underline hover:bg-accent hover:text-foreground aria-[current=page]:bg-sidebar-accent aria-[current=page]:font-medium aria-[current=page]:text-foreground aria-[current=page]:shadow-sm [&>svg]:size-4 [&>svg]:text-muted-foreground"
  return (
    <nav className="flex min-h-0 flex-1 flex-col gap-1" aria-label="Admin">
      <div className="flex min-h-12 items-center gap-3 px-3">
        <img className="size-8 flex-none" src="/static/mark.svg" alt="" />
        <span className="grid leading-tight">
          <strong className="font-heading text-sm font-semibold">Admin</strong>
          {shell?.orgName && (
            <span className="text-xs text-muted-foreground">
              {shell.orgName}
            </span>
          )}
        </span>
      </div>
      {shell?.sections.map((section) => (
        <AppLink
          key={section.key}
          href={section.href}
          className={item}
          aria-current={sectionOf(here) === section.key ? "page" : undefined}
          onClick={onGo}
        >
          {section.label}
        </AppLink>
      ))}
      <span className="flex-1" />
      {shell?.orgPage && (
        <a className={item} href={shell.orgPage}>
          <SettingsIcon /> Organization and billing
        </a>
      )}
      <a className={item} href="/">
        <ArrowLeftIcon /> Back to gusto
      </a>
      <form method="post" action="/logout" className="m-0">
        <button
          type="submit"
          className={cn(item, "cursor-pointer border-0 bg-transparent")}
        >
          <LogOutIcon /> Sign out
        </button>
      </form>
    </nav>
  )
}

// sectionOf is the console section a page belongs to.
function sectionOf(path: string): string {
  if (path === "/admin") return "overview"
  if (path.startsWith("/admin/usage") || path.startsWith("/admin/spending"))
    return "usage"
  if (
    path.startsWith("/admin/skill-sets/") ||
    path.startsWith("/admin/integrations/")
  )
    return "apps"
  return path.split("/")[2] ?? ""
}
