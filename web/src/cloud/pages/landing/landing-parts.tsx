import type { ComponentProps, ReactNode } from "react"

import type { CloudPage } from "@/cloud/types"
import { bookLabel, startHref, startLabel } from "@/domain/landing"
import { cn } from "@/lib/utils"

export const repository = "https://github.com/getlatentic/gusto"
export const docs = `${repository}/blob/main/docs`

// Cta is one of the landing page's calls to act: solid for starting, outlined for the rest.
export function Cta({
  solid,
  className,
  ...props
}: { solid?: boolean } & ComponentProps<"a">) {
  return (
    <a
      className={cn(
        "inline-block rounded-sm px-5 py-3 font-semibold no-underline",
        solid
          ? "bg-solid text-on-solid hover:bg-solid-hover"
          : "border-line-strong hover:bg-wash border",
        className
      )}
      {...props}
    />
  )
}

export function StartCta({
  page,
  className,
}: {
  page: CloudPage
  className?: string
}) {
  return (
    <Cta solid href={startHref(page)} className={className} data-testid="start">
      {startLabel(page)}
    </Cta>
  )
}

export function BookCta({
  page,
  className,
}: {
  page: CloudPage
  className?: string
}) {
  if (!page.Book) return null
  return (
    <Cta href={page.Book} className={className} data-testid="book">
      {bookLabel(page)}
    </Cta>
  )
}

// Band is one full-width section of the landing page, its ground alternating with the next.
export function Band({
  id,
  wash,
  className,
  children,
}: {
  id?: string
  wash?: boolean
  className?: string
  children: ReactNode
}) {
  return (
    <section
      id={id}
      className={cn(
        "text-ink py-14 min-[44rem]:py-20",
        wash ? "bg-wash" : "bg-canvas",
        className
      )}
    >
      <div className="mx-auto max-w-5xl px-5">{children}</div>
    </section>
  )
}

export function BandTitle({ children }: { children: ReactNode }) {
  return (
    <h2 className="m-0 text-[length:clamp(1.5rem,3vw,2rem)] leading-[1.25] font-semibold">
      {children}
    </h2>
  )
}

// Tile is one card in a band's grid: a title and a line or two.
export function Tile({
  title,
  children,
  className,
}: {
  title: ReactNode
  children?: ReactNode
  className?: string
}) {
  return (
    <li
      className={cn(
        "border-line-strong grid content-start gap-3 rounded-lg border bg-surface p-6",
        className
      )}
    >
      <h3 className="m-0 text-lg leading-[1.3] font-semibold">{title}</h3>
      {children}
    </li>
  )
}

export function TileText({ children }: { children: ReactNode }) {
  return <p className="text-md text-quiet m-0 leading-[1.6]">{children}</p>
}
