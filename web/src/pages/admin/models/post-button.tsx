import type { ComponentProps } from "react"

import { Button } from "@/components/ui/button"
import { withBase } from "@/lib/base"

// PostButton is one action as a form of its own: a button that posts its fields to the server, which goes back to this page.
export function PostButton({
  action,
  fields = {},
  children,
  ...button
}: {
  action: string
  fields?: Record<string, string>
} & Omit<ComponentProps<typeof Button>, "type">) {
  return (
    <form method="post" action={withBase(action)} className="m-0">
      {Object.entries(fields).map(([name, value]) => (
        <input key={name} type="hidden" name={name} value={value} />
      ))}
      <Button type="submit" {...button}>
        {children}
      </Button>
    </form>
  )
}
