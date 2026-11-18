import type { ReactNode } from "react"
import { ArrowLeftIcon, UsersIcon } from "lucide-react"

import { AppLink } from "@/components/app-link"
import { Loading } from "@/components/controls"
import { dataOf, type Loaded } from "@/lib/api"
import type { RoomLink } from "@/pages/members/types"

// The parts an agent's page and a person's page are both made of.

export const sectionTitle = "m-0 text-sm font-semibold"
export const emptyNote =
  "m-0 rounded-lg border border-dashed px-5 py-8 text-center text-sm text-subtle"

// Opening draws whoever the address names once their page is read, keeping the one before on screen while the next is read.
export function Opening<T>({
  loaded,
  children,
}: {
  loaded: Loaded<T>
  children: (page: T) => ReactNode
}) {
  const page = dataOf(loaded)
  if (page === undefined) {
    return (
      <div className="mx-auto max-w-3xl px-8 py-8 max-nav:px-4">
        <Loading loaded={loaded}>{() => null}</Loading>
      </div>
    )
  }
  return <div aria-busy={loaded.state === "loading"}>{children(page)}</div>
}

// BackToMembers is the way back to the list on a narrow window, where the list folds away while somebody is open.
export function BackToMembers() {
  return (
    <AppLink
      href="/members"
      className="hidden w-fit items-center gap-1.5 text-sm text-muted-foreground no-underline hover:text-foreground max-nav:inline-flex"
    >
      <ArrowLeftIcon aria-hidden="true" className="size-4" /> Members
    </AppLink>
  )
}

// Facts are what a page says of somebody, a term and its value a row.
export function Facts({ children }: { children: ReactNode }) {
  return <dl className="m-0 grid gap-px rounded-lg border p-1.5">{children}</dl>
}

export function Fact({
  term,
  children,
}: {
  term: ReactNode
  children: ReactNode
}) {
  return (
    <div className="grid min-h-10 grid-cols-[9rem_minmax(0,1fr)] items-center gap-3 px-2 py-1.5 max-sm:grid-cols-1 max-sm:gap-0.5">
      <dt className="flex items-center gap-2 text-sm text-muted-foreground [&>svg]:size-4">
        {term}
      </dt>
      <dd className="m-0 min-w-0 text-sm">{children}</dd>
    </div>
  )
}

// RoomLinks is a list of rooms under its title, and nothing when there are none.
export function RoomLinks({
  title,
  rooms,
}: {
  title: string
  rooms: RoomLink[]
}) {
  if (rooms.length === 0) return null
  return (
    <section aria-label={title} className="grid gap-3">
      <h3 className={sectionTitle}>{title}</h3>
      <ul className="m-0 grid list-none gap-px rounded-lg border p-1.5">
        {rooms.map((room) => (
          <li key={room.Where}>
            <AppLink
              href={room.Where}
              className="flex h-10 items-center gap-3 rounded-md px-2 text-sm text-foreground no-underline hover:bg-hover"
            >
              <UsersIcon aria-hidden="true" className="size-4 text-subtle" />
              {room.Name}
            </AppLink>
          </li>
        ))}
      </ul>
    </section>
  )
}
