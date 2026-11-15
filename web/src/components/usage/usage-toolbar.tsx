import { useState } from "react"
import { Link, useNavigate } from "react-router"
import type { DateRange } from "react-day-picker"
import { CalendarIcon, CheckIcon, PlusCircleIcon, XIcon } from "lucide-react"

import { SearchBox } from "@/components/search-box"
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { Separator } from "@/components/ui/separator"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { dayFromISO, isoDay } from "@/lib/format"
import type { UsageView } from "@/lib/types"
import { cn } from "@/lib/utils"

// The usage's toolbar narrows the conversations as each control changes, with no button to press: a search, a span of days, and one
// choice of each kind of who and what. Every choice is part of the page's address, so it can be kept
// and shared, and Reset goes back to all of them.
export function UsageToolbar({
  view,
  base,
}: {
  view: UsageView
  base: string
}) {
  const navigate = useNavigate()
  const value = (name: string) => view.Values[name]?.[0] ?? ""
  const go = (changes: Record<string, string>) => {
    const query = new URLSearchParams()
    for (const [name, values] of Object.entries(view.Values)) {
      if (name === "after" || name in changes) continue
      for (const each of values) query.append(name, each)
    }
    for (const [name, chosen] of Object.entries(changes)) {
      if (chosen) query.set(name, chosen)
    }
    const text = query.toString()
    navigate(text ? `${base}?${text}` : base)
  }
  const facets: { name: string; title: string; options: string[] | null }[] = [
    ...(view.Everyone
      ? [{ name: "person", title: "Person", options: view.Facets.people }]
      : []),
    { name: "agent", title: "Agent", options: view.Facets.agents },
    {
      name: "agent_type",
      title: "Agent type",
      options: view.Facets.agentTypes,
    },
    { name: "provider", title: "Provider", options: view.Facets.providers },
    { name: "model", title: "Model", options: view.Facets.models },
  ]
  const chosenSomething = view.Filtered || Boolean(value("from") || value("to"))
  return (
    <div className="mb-4 flex flex-wrap items-center gap-2" role="search">
      <SearchBox
        label="Search sessions, people, agents, agent types, providers and models"
        initial={value("q")}
        onSearch={(q) => go({ q })}
      />
      <DateRangeButton
        from={value("from")}
        to={value("to")}
        onChange={(from, to) => go({ from, to })}
      />
      {facets.map((facet) => (
        <FacetButton
          key={facet.name}
          title={facet.title}
          options={facet.options}
          chosen={value(facet.name)}
          onChoose={(chosen) => go({ [facet.name]: chosen })}
        />
      ))}
      {chosenSomething && (
        <Link
          to={view.Clear}
          className={buttonVariants({ variant: "ghost", size: "sm" })}
        >
          Reset <XIcon />
        </Link>
      )}
    </div>
  )
}

const shortMonths = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
]
const dayWords = (d: Date) => `${d.getDate()} ${shortMonths[d.getMonth()]}`

// rangeWords is a span of days as the button names it.
function rangeWords(from?: Date, to?: Date): string {
  if (from && to)
    return isoDay(from) === isoDay(to)
      ? dayWords(from)
      : `${dayWords(from)} – ${dayWords(to)}`
  if (from) return `From ${dayWords(from)}`
  if (to) return `Until ${dayWords(to)}`
  return "Any date"
}

function presets(today: Date): { label: string; from: Date; to: Date }[] {
  const daysAgo = (n: number) =>
    new Date(today.getFullYear(), today.getMonth(), today.getDate() - n)
  return [
    { label: "Today", from: today, to: today },
    { label: "Last 7 days", from: daysAgo(6), to: today },
    { label: "Last 30 days", from: daysAgo(29), to: today },
    {
      label: "This month",
      from: new Date(today.getFullYear(), today.getMonth(), 1),
      to: today,
    },
    {
      label: "Last month",
      from: new Date(today.getFullYear(), today.getMonth() - 1, 1),
      to: new Date(today.getFullYear(), today.getMonth(), 0),
    },
  ]
}

// DateRangeButton picks a span of days from two months of calendar, or one of the usual spans.
function DateRangeButton({
  from,
  to,
  onChange,
}: {
  from: string
  to: string
  onChange: (from: string, to: string) => void
}) {
  const [open, setOpen] = useState(false)
  const shown: DateRange = { from: dayFromISO(from), to: dayFromISO(to) }
  const [picking, setPicking] = useState<DateRange | undefined>(shown)
  const now = new Date()
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const choose = (range: DateRange | undefined) => {
    onChange(
      range?.from ? isoDay(range.from) : "",
      range?.to ? isoDay(range.to) : ""
    )
    setOpen(false)
  }
  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (next) setPicking(shown)
      }}
    >
      <PopoverTrigger
        render={
          <Button
            variant="outline"
            size="sm"
            className={cn(
              "font-normal",
              !shown.from && !shown.to && "text-muted-foreground"
            )}
          />
        }
      >
        <CalendarIcon />
        {rangeWords(shown.from, shown.to)}
      </PopoverTrigger>
      <PopoverContent
        className="w-auto flex-row gap-0 p-0 max-sm:flex-col"
        align="start"
      >
        <div className="flex flex-col gap-0.5 border-r p-2 max-sm:flex-row max-sm:flex-wrap max-sm:border-r-0 max-sm:border-b">
          {presets(today).map((preset) => (
            <Button
              key={preset.label}
              variant="ghost"
              size="sm"
              className="justify-start font-normal"
              onClick={() => choose({ from: preset.from, to: preset.to })}
            >
              {preset.label}
            </Button>
          ))}
          <Button
            variant="ghost"
            size="sm"
            className="justify-start font-normal"
            onClick={() => choose(undefined)}
          >
            Any date
          </Button>
        </div>
        <Calendar
          mode="range"
          numberOfMonths={2}
          defaultMonth={
            shown.from ?? new Date(today.getFullYear(), today.getMonth() - 1, 1)
          }
          selected={picking}
          disabled={{ after: today }}
          onSelect={(range) => {
            setPicking(range)
            if (
              range?.from &&
              range.to &&
              isoDay(range.from) !== isoDay(range.to)
            )
              choose(range)
          }}
        />
      </PopoverContent>
    </Popover>
  )
}

// FacetButton chooses one person, agent, agent type, provider or model from those the dates hold, searching the list as it is typed.
function FacetButton({
  title,
  options,
  chosen,
  onChoose,
}: {
  title: string
  options: string[] | null
  chosen: string
  onChoose: (chosen: string) => void
}) {
  const [open, setOpen] = useState(false)
  const pick = (next: string) => {
    onChoose(next === chosen ? "" : next)
    setOpen(false)
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            variant="outline"
            size="sm"
            className="border-dashed font-normal"
          />
        }
      >
        <PlusCircleIcon />
        {title}
        {chosen && (
          <>
            <Separator orientation="vertical" className="mx-0.5 h-4" />
            <Badge
              variant="secondary"
              className="max-w-40 truncate rounded-sm px-1.5 font-normal"
            >
              {chosen}
            </Badge>
          </>
        )}
      </PopoverTrigger>
      <PopoverContent className="w-60 p-0" align="start">
        <Command>
          <CommandInput placeholder={title} />
          <CommandList>
            <CommandEmpty>None found.</CommandEmpty>
            <CommandGroup>
              {(options ?? []).map((option) => (
                <CommandItem
                  key={option}
                  value={option}
                  onSelect={() => pick(option)}
                >
                  <CheckIcon
                    className={cn(
                      option === chosen ? "opacity-100" : "opacity-0"
                    )}
                  />
                  <span className="truncate">{option}</span>
                </CommandItem>
              ))}
            </CommandGroup>
            {chosen && (
              <>
                <CommandSeparator />
                <CommandGroup>
                  <CommandItem
                    onSelect={() => pick(chosen)}
                    className="justify-center"
                  >
                    Clear
                  </CommandItem>
                </CommandGroup>
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

// StateToggle shows every conversation, or only those whose latest turn failed, with how many failed this week.
export function StateToggle({ states }: { states: UsageView["States"] }) {
  const navigate = useNavigate()
  const current = states.find((state) => state.Current)?.Label ?? "All"
  return (
    <ToggleGroup
      variant="outline"
      size="sm"
      value={[current]}
      aria-label="Which conversations"
      onValueChange={(next) => {
        const state = states.find((each) => each.Label === next[0])
        if (state && !state.Current) navigate(state.Href)
      }}
    >
      {states.map((state) => (
        <ToggleGroupItem
          key={state.Label}
          value={state.Label}
          title={state.Count ? `${state.Label}: ${state.Count}` : undefined}
          className="px-3 font-normal data-pressed:font-medium"
        >
          {state.Label}
          {state.Count && !state.Count.startsWith("0 ") && (
            <span className="text-muted-foreground">
              {state.Count.replace(" this week", "")}
            </span>
          )}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )
}
