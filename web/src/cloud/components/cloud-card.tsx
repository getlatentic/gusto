import type { ComponentProps, ReactNode } from "react"

import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { cn } from "@/lib/utils"

// CloudCard is one of gusto cloud's pages that is a single step: a card in the middle of the window. A wide card has the room for
// a booking calendar.
export function CloudCard({
  wide,
  className,
  children,
}: {
  wide?: boolean
  className?: string
  children: ReactNode
}) {
  return (
    <main className="grid min-h-dvh place-items-center bg-background p-4">
      <Card
        className={cn(
          "w-full shadow-md",
          wide ? "max-w-[64rem]" : "max-w-[26rem]",
          className
        )}
      >
        <CardContent className="grid gap-6 px-8 py-2 max-[26rem]:px-5">
          {children}
        </CardContent>
      </Card>
    </main>
  )
}

export function Brand() {
  return (
    <p className="m-0 flex items-center gap-3 font-heading text-lg leading-none font-semibold">
      <img className="h-auto w-7" src="/static/mark.svg" alt="" /> gusto
    </p>
  )
}

// BrandBar is the brand with the way out beside it, for a page a person reaches signed in.
export function BrandBar() {
  return (
    <header className="flex items-center justify-between">
      <Brand />
      <SignOut />
    </header>
  )
}

export function SignOut({ className }: { className?: string }) {
  return (
    <form method="post" action="/signout" className={className}>
      <Button variant="outline" type="submit" data-testid="sign-out">
        Sign out
      </Button>
    </form>
  )
}

export function Heading({ children }: { children: ReactNode }) {
  return <h1 className="m-0 text-xl font-semibold">{children}</h1>
}

// Refusal is why a form was not taken, said where the person reads it first.
export function Refusal({ children }: { children: ReactNode }) {
  if (!children) return null
  return (
    <p className="m-0 text-sm text-bad" role="alert" data-testid="refusal">
      {children}
    </p>
  )
}

export function Quiet({ className, ...props }: ComponentProps<"p">) {
  return (
    <p
      className={cn("m-0 text-sm text-muted-foreground", className)}
      {...props}
    />
  )
}

// Waiting is something under way, with the spinner that says so.
export function Waiting({ children, ...props }: ComponentProps<"p">) {
  return (
    <p
      className="m-0 inline-flex items-center gap-3 before:size-4 before:flex-none before:animate-spin before:rounded-full before:border-2 before:border-primary before:border-t-transparent before:content-['']"
      {...props}
    >
      {children}
    </p>
  )
}

export function TextLink({ className, ...props }: ComponentProps<"a">) {
  return (
    <a
      className={cn("underline hover:text-foreground", className)}
      {...props}
    />
  )
}
