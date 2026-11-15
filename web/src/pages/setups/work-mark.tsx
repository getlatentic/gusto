import { AgentIcon } from "@/components/agent-icon"

// The drawing of each field of work, as the presets carry it.
const drawings: Record<string, string> = {
  Health: "M3 12h4l2-5 4 10 2-5h6",
  Education: "M3 7l9-4 9 4-9 4-9-4zM7 9v5c0 1.5 2.2 3 5 3s5-1.5 5-3V9",
  HR: "M9 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM3 20c0-3 2.7-5 6-5s6 2 6 5M16 11a2.5 2.5 0 1 0 0-5M18 15c2 .5 3 2.2 3 4",
  Defense: "M12 3l8 3v6c0 4.5-3.4 8-8 9-4.6-1-8-4.5-8-9V6l8-3z",
  Engineering: "M8 8l-4 4 4 4M16 8l4 4-4 4M13.5 5l-3 14",
}

// WorkMark is what stands for a kind of work at the head of its page: the agent type's logo, the drawing of its field of work,
// or its initial.
export function WorkMark({
  mark,
  field,
  name,
}: {
  mark?: string
  field?: string
  name: string
}) {
  if (mark) return <AgentIcon src={mark} className="size-10" />
  return (
    <span
      className="grid size-16 flex-none place-items-center rounded-lg bg-muted text-muted-foreground ring-1 ring-border ring-inset"
      aria-hidden="true"
    >
      {field ? (
        <svg
          viewBox="0 0 24 24"
          className="size-[55%] fill-none stroke-current stroke-[1.7] [stroke-linecap:round] [stroke-linejoin:round]"
        >
          <path d={drawings[field] ?? "M12 3v18M3 12h18"} />
        </svg>
      ) : (
        <span className="font-heading text-sm leading-none font-semibold">
          {name.charAt(0).toUpperCase()}
        </span>
      )}
    </span>
  )
}
