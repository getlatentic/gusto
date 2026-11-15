import {
  ActivityIcon,
  CodeXmlIcon,
  GraduationCapIcon,
  PlusIcon,
  ShieldIcon,
  UsersIcon,
} from "lucide-react"

import { AgentIcon } from "@/components/agent-icon"
import { initial } from "@/lib/format"

// The drawing of the kind of work an agent was set up for, for an agent whose type has no mark.
const fields = {
  Health: ActivityIcon,
  Education: GraduationCapIcon,
  HR: UsersIcon,
  Defense: ShieldIcon,
  Engineering: CodeXmlIcon,
}

// ProjectMark is an agent's mark at the head of its page: its type's logo, the drawing of its field, or its initial.
export function ProjectMark({
  mark,
  field,
  name,
}: {
  mark: string
  field: string
  name: string
}) {
  if (mark) return <AgentIcon src={mark} className="size-10" />
  const Drawing = field
    ? (fields[field as keyof typeof fields] ?? PlusIcon)
    : null
  return (
    <span
      className="grid size-16 flex-none place-items-center rounded-lg bg-muted text-muted-foreground ring-1 ring-border ring-inset"
      aria-hidden="true"
    >
      {Drawing ? (
        <Drawing className="size-[55%]" strokeWidth={1.7} />
      ) : (
        <span className="text-sm leading-none font-semibold">
          {initial(name)}
        </span>
      )}
    </span>
  )
}
