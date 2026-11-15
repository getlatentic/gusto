import { Fragment, type ReactNode } from "react"
import { MenuIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { useNavigation } from "@/components/navigation"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

// A page's bar runs across the top of the pane: the pages above it, its title, and its few actions. On a narrow window it
// carries the button that opens the side list.
export function PageBar({
  title,
  crumbs = [],
  children,
}: {
  title: ReactNode
  crumbs?: { label: string; href: string }[]
  children?: ReactNode
}) {
  const navigation = useNavigation()
  return (
    <header className="sticky top-0 z-20 flex h-16 flex-none items-center gap-3 bg-background/92 px-4 backdrop-blur-xl max-md:px-3">
      <Button
        variant="ghost"
        size="icon-sm"
        className="md:hidden"
        aria-label="Menu"
        onClick={navigation.open}
      >
        <MenuIcon />
      </Button>
      <div className="flex min-w-0 flex-1 items-center gap-1.5 text-sm">
        {crumbs.length > 0 && (
          <nav className="contents" aria-label="Where this is">
            {crumbs.map((crumb) => (
              <Fragment key={crumb.href}>
                <AppLink
                  href={crumb.href}
                  className="max-w-48 min-w-0 flex-none truncate text-muted-foreground no-underline hover:text-foreground max-sm:hidden"
                >
                  {crumb.label}
                </AppLink>
                <span
                  className="flex-none text-faint max-sm:hidden"
                  aria-hidden="true"
                >
                  /
                </span>
              </Fragment>
            ))}
          </nav>
        )}
        <h1 className="m-0 min-w-0 truncate font-sans text-sm font-semibold">
          {title}
        </h1>
      </div>
      {children}
    </header>
  )
}

// A page's column: wide, for the admin's tables of everybody.
export function PageBody({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        "mx-auto w-full max-w-6xl px-8 pt-4 pb-20 max-md:px-4 max-md:pb-16",
        className
      )}
    >
      {children}
    </div>
  )
}

// The line at the top of a page saying what it is for.
export function PageSub({ children }: { children: ReactNode }) {
  return (
    <p className="m-0 mb-6 max-w-[68ch] text-sm text-pretty text-muted-foreground">
      {children}
    </p>
  )
}

// A notice says what stops the page from being whole; a status says what is still being done.
export function Notice({
  children,
  tone = "bad",
}: {
  children: ReactNode
  tone?: "bad" | "warn" | "plain"
}) {
  return (
    <p
      role={tone === "bad" ? "alert" : "status"}
      className={cn(
        "m-0 mb-4 rounded-md px-4 py-3 text-sm",
        tone === "bad" && "bg-bad-wash text-bad",
        tone === "warn" && "bg-warn-wash",
        tone === "plain" && "bg-secondary"
      )}
    >
      {children}
    </p>
  )
}
