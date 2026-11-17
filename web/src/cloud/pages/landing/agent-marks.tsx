import { AgentIcon } from "@/components/agent-icon"

const agents: { name: string; mark: string }[] = [
  { name: "Claude Code", mark: "/static/agents/claude.svg" },
  { name: "Codex", mark: "/static/agents/codex.svg" },
  { name: "OpenCode", mark: "/static/agents/opencode.svg" },
  { name: "Pi", mark: "/static/agents/pi.svg" },
  { name: "Hermes", mark: "/static/agents/hermes.webp" },
  { name: "DeepSeek", mark: "/static/agents/dsh.svg" },
]

// AgentMarks is the agents gusto runs, as a row of their marks with their names under them.
export function AgentMarks() {
  return (
    <ul
      aria-label="Agents"
      data-testid="agents"
      className="m-0 mt-10 grid list-none grid-cols-3 gap-x-4 gap-y-8 p-0 min-[44rem]:grid-cols-6"
    >
      {agents.map((agent) => (
        <li
          key={agent.name}
          data-reveal
          className="grid justify-items-center gap-3 text-center text-sm font-medium"
        >
          <span className="border-line grid size-16 place-items-center rounded-2xl border bg-surface">
            <AgentIcon src={agent.mark} className="size-8" />
          </span>
          {agent.name}
        </li>
      ))}
    </ul>
  )
}
