import type { ReactNode } from "react"

import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"

// Section is one titled part of an organization's page.
export function Section({
  title,
  id,
  children,
}: {
  title: string
  id?: string
  children: ReactNode
}) {
  return (
    <Card className="py-5" id={id} aria-labelledby={id && `${id}-title`}>
      <CardContent className="grid gap-3 px-5">
        <h2 id={id && `${id}-title`} className="m-0 text-sm font-semibold">
          {title}
        </h2>
        {children}
      </CardContent>
    </Card>
  )
}

// A small form that sends one change about one person, such as their role.
export function PersonForm({
  action,
  fields,
  children,
}: {
  action: string
  fields: Record<string, string>
  children: ReactNode
}) {
  return (
    <form method="post" action={action}>
      {Object.entries(fields).map(([name, value]) => (
        <input key={name} type="hidden" name={name} value={value} />
      ))}
      <Button variant="outline" size="sm" type="submit">
        {children}
      </Button>
    </form>
  )
}
