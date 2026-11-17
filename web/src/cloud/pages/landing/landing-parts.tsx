import type { ComponentProps, ReactNode } from "react"

import type { CloudPage } from "@/cloud/types"
import { cn } from "@/lib/utils"

export const repository = "https://github.com/getlatentic/gusto"
export const docs = `${repository}/blob/main/docs`

// Cta is one of the landing page's calls to act, all one size: solid for starting, outlined for the rest.
export function Cta({
  solid,
  className,
  ...props
}: { solid?: boolean } & ComponentProps<"a">) {
  return (
    <a
      className={cn(
        "inline-flex h-10 items-center justify-center rounded-md px-4 text-sm font-medium whitespace-nowrap no-underline",
        solid
          ? "bg-solid text-on-solid hover:bg-solid-hover"
          : "border-line-strong text-ink hover:bg-wash border bg-surface",
        className
      )}
      {...props}
    />
  )
}

export function StartCta({ className }: { className?: string }) {
  return (
    <Cta solid href="/signup" className={className} data-testid="start">
      Get started
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
      Talk to us
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
      <div className="mx-auto max-w-6xl px-5">{children}</div>
    </section>
  )
}

export function BandTitle({ children }: { children: ReactNode }) {
  return (
    <h2
      className="m-0 text-[length:clamp(1.5rem,3vw,2rem)] leading-[1.25] font-semibold"
      data-reveal
    >
      {children}
    </h2>
  )
}

export function BandText({ children }: { children: ReactNode }) {
  return <p className="text-quiet m-0 text-sm">{children}</p>
}
