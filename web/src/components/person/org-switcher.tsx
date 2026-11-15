import { useState } from "react"
import { ArrowUpRightIcon, CheckIcon } from "lucide-react"
import { useQuery } from "@tanstack/react-query"

import { buttonVariants } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { orgLines, type CloudOrg } from "@/domain/orgs"
import { askCloud } from "@/lib/cloud"
import type { PersonShell } from "@/domain/side-list"
import { cn } from "@/lib/utils"

const line = cn(
  buttonVariants({ variant: "ghost", size: "sm" }),
  "w-full justify-start font-normal"
)

// OrgSwitcher is the mark at the top of the rail behind gusto cloud: this organization, and every other the person is in,
// asked of gusto cloud when the menu first opens. Until it answers, or if it cannot, the menu shows this organization alone.
export function OrgSwitcher({ shell }: { shell: PersonShell }) {
  const org = shell.Org!
  const [asked, setAsked] = useState(false)
  const listed = useQuery({
    queryKey: [org.OrgsFrom],
    queryFn: ({ signal }) =>
      askCloud<{ orgs?: CloudOrg[] }>(org.OrgsFrom, signal),
    enabled: asked,
    staleTime: 60_000,
  })
  const orgs = listed.data?.orgs ?? []
  const lines = orgs.length
    ? orgLines(orgs, org.Slug, shell.Workspace)
    : [
        {
          kind: "place" as const,
          key: org.Slug,
          text: shell.Workspaces
            ? `${org.Name} · ${shell.Workspaces.Here.Name}`
            : org.Name,
          href: "",
          current: true,
          inset: false,
        },
      ]
  return (
    <Popover onOpenChange={(open) => open && setAsked(true)}>
      <PopoverTrigger
        className="mb-2 grid size-10 cursor-pointer place-items-center rounded-md hover:bg-hover"
        aria-label="Organizations"
      >
        <img className="size-6" src="/static/mark.svg" alt="" />
      </PopoverTrigger>
      <PopoverContent
        side="right"
        align="start"
        className="grid w-64 gap-1 p-1"
      >
        <ul
          className="m-0 grid list-none gap-0.5 p-0"
          aria-label="Organizations"
        >
          {lines.map((each) =>
            each.kind === "heading" ? (
              <li
                key={each.key}
                className="truncate px-2.5 pt-2 pb-1 text-xs font-semibold text-muted-foreground"
              >
                {each.text}
              </li>
            ) : (
              <li key={each.key}>
                {each.current ? (
                  <span
                    className={cn(line, "bg-hover", each.inset && "pl-6")}
                    aria-current="true"
                  >
                    <span className="min-w-0 flex-1 truncate">{each.text}</span>
                    <CheckIcon />
                  </span>
                ) : (
                  <a
                    className={cn(line, each.inset && "pl-6")}
                    href={each.href}
                  >
                    <span className="min-w-0 flex-1 truncate">{each.text}</span>
                  </a>
                )}
              </li>
            )
          )}
        </ul>
        <hr className="my-1 border-border" />
        <a className={line} href={org.Manage}>
          <ArrowUpRightIcon /> Manage organizations
        </a>
      </PopoverContent>
    </Popover>
  )
}
