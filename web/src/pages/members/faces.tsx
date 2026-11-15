import { UserIcon } from "lucide-react"

import { AgentIcon } from "@/components/agent-icon"
import { initial } from "@/lib/format"
import { cn } from "@/lib/utils"

const sizes = {
  sm: { face: "size-6 text-[0.625rem]", mark: "size-4" },
  md: { face: "size-9 text-xs", mark: "size-5" },
  lg: { face: "size-14 text-sm", mark: "size-8" },
} as const

type Size = keyof typeof sizes

// PersonFace is a person's initials in a circle, or a figure for somebody with none, as the shared sign-in.
export function PersonFace({
  initials,
  size = "md",
}: {
  initials: string
  size?: Size
}) {
  return (
    <span
      className={cn(
        "grid flex-none place-items-center rounded-full bg-sunken font-medium text-muted-foreground",
        sizes[size].face
      )}
      aria-hidden="true"
    >
      {initials || <UserIcon className="size-[45%]" />}
    </span>
  )
}

// AgentFace is an agent's mark in a circle: its agent type's logo, or the agent's initial where the type has none.
export function AgentFace({
  mark,
  name,
  size = "md",
}: {
  mark: string
  name: string
  size?: Size
}) {
  return (
    <span
      className={cn(
        "grid flex-none place-items-center overflow-hidden rounded-full bg-sunken font-medium text-muted-foreground",
        sizes[size].face
      )}
      aria-hidden="true"
    >
      {mark ? (
        <AgentIcon src={mark} className={sizes[size].mark} />
      ) : (
        initial(name)
      )}
    </span>
  )
}
