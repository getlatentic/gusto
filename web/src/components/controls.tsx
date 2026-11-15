import { useState, type ReactNode } from "react"
import { Link } from "react-router"
import { CalendarIcon, XIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { Button, buttonVariants } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { Loaded } from "@/lib/api"
import { dayFromISO, isoDay, shortDay } from "@/lib/format"
import { cn } from "@/lib/utils"

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

// DateField is a day picked from a calendar, or none. It shows the day in words and gives it on as an ISO day.
export function DateField({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (day: string) => void
}) {
  const [open, setOpen] = useState(false)
  const day = dayFromISO(value)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            variant="outline"
            className={cn(
              "w-40 justify-start font-normal",
              !day && "text-muted-foreground"
            )}
            aria-label={label}
          >
            <CalendarIcon />
            {day ? shortDay(day) : label}
          </Button>
        }
      />
      <PopoverContent className="w-auto p-0" align="start">
        <Calendar
          mode="single"
          selected={day}
          defaultMonth={day}
          onSelect={(picked) => {
            onChange(picked ? isoDay(picked) : "")
            setOpen(false)
          }}
        />
        {day && (
          <div className="border-t p-2">
            <Button
              variant="ghost"
              size="sm"
              className="w-full"
              onClick={() => {
                onChange("")
                setOpen(false)
              }}
            >
              <XIcon /> No date
            </Button>
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}

// ChoiceSelect is one of a list, or all of them: the empty choice is "all".
export function ChoiceSelect({
  label,
  all,
  options,
  value,
  onChange,
}: {
  label: string
  all: string
  options: string[] | null
  value: string
  onChange: (value: string) => void
}) {
  const items = [
    { value: null as string | null, label: all },
    ...(options ?? []).map((option) => ({
      value: option as string | null,
      label: option,
    })),
  ]
  return (
    <Select
      items={items}
      value={value || null}
      onValueChange={(chosen) => onChange((chosen as string | null) ?? "")}
    >
      <SelectTrigger aria-label={label} className="min-w-40">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {items.map((item) => (
          <SelectItem key={item.label} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

// AgentMark is an agent type's logo, the dark one on a dark page when its owner publishes one, or its initial.
export function AgentMark({
  mark,
  markDark,
  name,
}: {
  mark: string
  markDark: string
  name: string
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
          "size-3.5 flex-none object-contain",
          markDark && "dark:hidden"
        )}
        src={mark}
        alt=""
        aria-hidden="true"
      />
      {markDark && (
        <img
          className="hidden size-3.5 flex-none object-contain dark:block"
          src={markDark}
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
