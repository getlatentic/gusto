import { useState } from "react"
import { ActivityIcon, LogOutIcon, SlidersHorizontalIcon } from "lucide-react"

import { buttonVariants } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import type { PersonShell } from "@/domain/side-list"
import {
  accents,
  readLook,
  setAccent,
  setTheme,
  stepZoom,
  type Theme,
} from "@/lib/look"
import { cn } from "@/lib/utils"
import { withBase } from "@/lib/base"

const themes: { value: Theme; label: string }[] = [
  { value: "system", label: "System" },
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
]

const item = cn(
  buttonVariants({ variant: "ghost" }),
  "w-full justify-start gap-3 font-medium"
)

// AccountMenu is who is signed in, at the foot of the rail: their activity, the console for an admin, how the pages look on
// this device, and signing out.
export function AccountMenu({ shell }: { shell: PersonShell }) {
  const [look, setLook] = useState(readLook)
  const who = shell.SignedAs
  return (
    <Popover onOpenChange={(open) => open && setLook(readLook())}>
      <PopoverTrigger
        className="mt-1 grid size-10 cursor-pointer place-items-center rounded-full hover:bg-hover aria-expanded:bg-hover"
        aria-label={`${who.Name}, account and settings`}
        title={who.Who}
      >
        <span
          className="grid size-8 place-items-center rounded-full bg-surface text-xs font-semibold shadow-sm"
          aria-hidden="true"
        >
          {who.Initials}
        </span>
      </PopoverTrigger>
      <PopoverContent side="right" align="end" className="grid w-64 gap-4">
        <nav aria-label="Account" className="grid gap-0.5">
          <a className={item} href={withBase("/jobs")}>
            <ActivityIcon /> Activity
          </a>
          {shell.Admin && (
            <a className={item} href={withBase("/admin")}>
              <SlidersHorizontalIcon /> Admin
            </a>
          )}
        </nav>
        <fieldset className="m-0 grid gap-2 border-0 p-0">
          <legend className="mb-2 text-xs font-semibold text-muted-foreground">
            Theme
          </legend>
          <ToggleGroup
            value={[look.theme]}
            onValueChange={(next) => {
              const chosen = next[0] as Theme | undefined
              if (chosen) setLook(setTheme(chosen))
            }}
            className="w-full"
          >
            {themes.map((theme) => (
              <ToggleGroupItem
                key={theme.value}
                value={theme.value}
                className="flex-1"
              >
                {theme.label}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </fieldset>
        <fieldset className="m-0 border-0 p-0">
          <legend className="mb-2 text-xs font-semibold text-muted-foreground">
            Text size
          </legend>
          <div className="flex items-center gap-2">
            <button
              type="button"
              className={buttonVariants({ variant: "outline", size: "sm" })}
              aria-label="Smaller"
              onClick={() => setLook(stepZoom(-1))}
            >
              A−
            </button>
            <output
              className="flex-1 text-center text-xs text-muted-foreground tabular-nums"
              aria-live="polite"
            >
              {look.zoom}%
            </output>
            <button
              type="button"
              className={buttonVariants({ variant: "outline", size: "sm" })}
              aria-label="Larger"
              onClick={() => setLook(stepZoom(1))}
            >
              A+
            </button>
          </div>
        </fieldset>
        <fieldset className="m-0 border-0 p-0">
          <legend className="mb-2 text-xs font-semibold text-muted-foreground">
            Accent
          </legend>
          <div className="flex justify-between">
            {accents.map((accent) => (
              <button
                key={accent.key}
                type="button"
                aria-label={accent.label}
                title={accent.label}
                aria-pressed={look.accent === accent.key}
                className="size-6.5 flex-none cursor-pointer rounded-full shadow-[inset_0_0_0_0.0625rem_rgb(0_0_0/0.12)] aria-pressed:shadow-[0_0_0_0.125rem_var(--gu-surface),0_0_0_0.25rem_var(--gu-text)]"
                style={{ background: accent.swatch }}
                onClick={() => setLook(setAccent(accent.key))}
              />
            ))}
          </div>
        </fieldset>
        <form
          method="post"
          action={withBase("/logout")}
          className="m-0 border-t pt-3"
        >
          <button type="submit" className={item}>
            <LogOutIcon /> Sign out
          </button>
        </form>
      </PopoverContent>
    </Popover>
  )
}
