import { type ReactNode } from "react"
import { Link } from "react-router"

import { AppLink } from "@/components/app-link"
import { buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { Loaded } from "@/lib/api"
import { cn } from "@/lib/utils"
import { withBase } from "@/lib/base"

// LinkTabs are the views of one page, each its own address, drawn as shadcn's line tabs.
export function LinkTabs({
  tabs,
  current,
  label,
}: {
  tabs: { key: string; label: ReactNode; href: string }[]
  current: string
  label: string
}) {
  return (
    <Tabs value={current} className="mb-5">
      <TabsList
        variant="line"
        aria-label={label}
        className="w-full justify-start border-b"
      >
        {tabs.map((tab) => (
          <TabsTrigger
            key={tab.key}
            value={tab.key}
            nativeButton={false}
            render={<Link to={tab.href} />}
            className="flex-none"
          >
            {tab.label}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  )
}

// AgentMark is an agent type's logo, the dark one on a dark page when its owner publishes one, or its initial. A logo is drawn
// as its owner made it, with nothing around it; size is its box, which it fills keeping its shape.
export function AgentMark({
  mark,
  markDark,
  name,
  size = "size-3.5",
}: {
  mark: string
  markDark: string
  name: string
  size?: string
}) {
  if (!mark) {
    return (
      <span
        className="grid size-3.5 flex-none place-items-center rounded-xs bg-muted text-[0.5rem] font-semibold text-muted-foreground"
        aria-hidden="true"
      >
        {name.charAt(0).toUpperCase()}
      </span>
    )
  }
  return (
    <>
      <img
        className={cn(
          size,
          "flex-none object-contain",
          markDark && "dark:hidden"
        )}
        src={withBase(mark)}
        alt=""
        aria-hidden="true"
      />
      {markDark && (
        <img
          className={cn(size, "hidden flex-none object-contain dark:block")}
          src={withBase(markDark)}
          alt=""
          aria-hidden="true"
        />
      )}
    </>
  )
}

// Pager is the way to the pages either side of a long list.
export function Pager({
  newer,
  older,
  newerLabel = "Newest",
  olderLabel = "Older",
  shown,
}: {
  newer?: string
  older?: string
  newerLabel?: string
  olderLabel?: string
  shown?: string
}) {
  if (!newer && !older) return null
  return (
    <nav
      className="flex items-center gap-3 border-t px-4 py-3 text-sm"
      aria-label="Pages"
    >
      {newer && (
        <AppLink
          href={newer}
          className={buttonVariants({ variant: "outline", size: "sm" })}
        >
          {newerLabel}
        </AppLink>
      )}
      <span className="flex-1 text-center text-muted-foreground tabular-nums">
        {shown}
      </span>
      {older && (
        <AppLink
          href={older}
          rel="next"
          className={buttonVariants({ variant: "outline", size: "sm" })}
        >
          {olderLabel}
        </AppLink>
      )}
    </nav>
  )
}

// Loading draws a page while its data comes, and says why when it could not.
export function Loading<T>({
  loaded,
  children,
}: {
  loaded: Loaded<T>
  children: (data: T) => ReactNode
}) {
  if (loaded.state === "failed") {
    return (
      <p
        role="alert"
        className="m-0 rounded-md bg-bad-wash px-4 py-3 text-sm text-bad"
      >
        {loaded.status === 404
          ? "There is nothing here."
          : `This could not be read: ${loaded.message}`}
      </p>
    )
  }
  if (loaded.data === undefined) {
    return (
      <div className="grid gap-3" aria-busy="true" aria-label="Loading">
        <Skeleton className="h-5 w-72" />
        <Skeleton className="h-32 w-full" />
        <Skeleton className="h-48 w-full" />
      </div>
    )
  }
  return (
    <div aria-busy={loaded.state === "loading"}>{children(loaded.data)}</div>
  )
}
