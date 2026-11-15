import type { ReactNode } from "react"
import {
  FolderIcon,
  LinkIcon,
  MessageSquareIcon,
  ScrollTextIcon,
  UsersIcon,
} from "lucide-react"

import { AppLink } from "@/components/app-link"
import { AccountMenu } from "@/components/person/account-menu"
import { OrgSwitcher } from "@/components/person/org-switcher"
import { placeFor, type Place } from "@/components/person/places"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { PersonShell } from "@/domain/side-list"

const placeLink =
  "grid size-10 place-items-center rounded-md text-faint no-underline transition-colors hover:bg-hover hover:text-foreground aria-[current=page]:bg-hover aria-[current=page]:text-foreground [&_svg]:size-4"

// PlacesRail is the narrow column of a person's places: chat, files, members, usage, and their account at its foot.
export function PlacesRail({
  shell,
  here,
  onGo,
}: {
  shell?: PersonShell
  here: string
  onGo?: () => void
}) {
  const current = placeFor(here)
  return (
    <nav
      aria-label="Places"
      className="flex w-13 flex-none flex-col items-center gap-1 border-r bg-rail py-3"
    >
      {shell?.Org ? (
        <OrgSwitcher shell={shell} />
      ) : (
        <AppLink
          href="/chat"
          aria-label="gusto"
          className="mb-2 grid size-10 place-items-center"
        >
          <img className="size-6" src="/static/mark.svg" alt="" />
        </AppLink>
      )}
      <PlaceLink
        place="chat"
        href="/chat"
        label="Chat"
        current={current}
        onGo={onGo}
      >
        <MessageSquareIcon />
      </PlaceLink>
      <PlaceLink
        place="files"
        href="/files"
        label="Files"
        current={current}
        onGo={onGo}
      >
        <FolderIcon />
      </PlaceLink>
      <PlaceLink
        place="members"
        href="/members"
        label="Members"
        current={current}
        onGo={onGo}
      >
        <UsersIcon />
      </PlaceLink>
      <PlaceLink
        place="usage"
        href="/usage"
        label="Usage"
        current={current}
        onGo={onGo}
      >
        <ScrollTextIcon />
      </PlaceLink>
      <span className="flex-1" />
      <span className="mb-1 h-px w-6 bg-input" aria-hidden="true" />
      {shell?.Connections && (
        <PlaceLink
          place="connections"
          href="/connections"
          label="Connections"
          current={current}
          onGo={onGo}
        >
          <LinkIcon />
        </PlaceLink>
      )}
      {shell && <AccountMenu shell={shell} />}
    </nav>
  )
}

function PlaceLink({
  place,
  href,
  label,
  current,
  onGo,
  children,
}: {
  place: Place
  href: string
  label: string
  current?: Place
  onGo?: () => void
  children: ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <AppLink
            href={href}
            aria-label={label}
            aria-current={current === place ? "page" : undefined}
            className={placeLink}
            onClick={onGo}
          />
        }
      >
        {children}
      </TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  )
}
