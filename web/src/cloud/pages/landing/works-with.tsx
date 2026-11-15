import { useState } from "react"

import { Band, BandTitle } from "@/cloud/pages/landing/landing-parts"
import { AgentIcon } from "@/components/agent-icon"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

type Kind = "agent" | "model" | "place"

interface Thing {
  name: string
  kind: Kind
  mark?: string
}

const things: Thing[] = [
  { name: "Claude Code", kind: "agent", mark: "/static/agents/claude.svg" },
  { name: "Codex", kind: "agent", mark: "/static/agents/codex.svg" },
  { name: "OpenCode", kind: "agent", mark: "/static/agents/opencode.svg" },
  { name: "Pi", kind: "agent", mark: "/static/agents/pi.svg" },
  { name: "Hermes Agent", kind: "agent", mark: "/static/agents/hermes.webp" },
  { name: "DeepSeek Agent", kind: "agent", mark: "/static/agents/dsh.svg" },
  { name: "Your own model key", kind: "model" },
  { name: "gusto’s models", kind: "model" },
  { name: "Your laptop", kind: "place" },
  { name: "Your own cloud", kind: "place" },
  { name: "gusto cloud", kind: "place" },
]

const kindWord: Record<Kind, string> = {
  agent: "Agent",
  model: "Model",
  place: "Place",
}

const filters: { value: Kind | "all"; label: string }[] = [
  { value: "all", label: "All" },
  { value: "agent", label: "Agents" },
  { value: "model", label: "Models" },
  { value: "place", label: "Where it runs" },
]

// WorksWith is what gusto runs with, which a visitor narrows to agents, models or places.
export function WorksWith() {
  const [shown, setShown] = useState<Kind | "all">("all")
  return (
    <Band id="works-with">
      <BandTitle>Works with your agents, models and computers</BandTitle>
      <ToggleGroup
        className="mt-8 flex-wrap gap-2"
        value={[shown]}
        onValueChange={(next: string[]) =>
          setShown((next[0] as Kind | "all") ?? "all")
        }
        aria-label="Show"
        data-testid="works-with-filter"
      >
        {filters.map((filter) => (
          <ToggleGroupItem
            key={filter.value}
            value={filter.value}
            className="border-line-strong text-md data-[pressed]:bg-solid data-[pressed]:text-on-solid rounded-full border px-4 data-[pressed]:border-transparent"
          >
            {filter.label}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      <ul
        className="m-0 mt-6 grid list-none grid-cols-2 gap-3 p-0 min-[44rem]:grid-cols-3 min-[64rem]:grid-cols-4"
        data-testid="works-with"
      >
        {things
          .filter((thing) => shown === "all" || thing.kind === shown)
          .map((thing) => (
            <li
              key={thing.name}
              className="border-line-strong grid content-start gap-1 rounded-lg border bg-surface p-4"
            >
              <span className="text-copy flex items-center gap-3 font-heading leading-tight font-semibold">
                {thing.mark && (
                  <span className="inline-grid size-7 flex-none place-items-center">
                    <AgentIcon src={thing.mark} className="size-6" />
                  </span>
                )}
                {thing.name}
              </span>
              <span className="text-quiet text-sm">{kindWord[thing.kind]}</span>
            </li>
          ))}
      </ul>
    </Band>
  )
}
