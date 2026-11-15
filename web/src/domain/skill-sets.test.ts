import { describe, expect, it } from "vitest"

import { holds, serversLine } from "@/domain/skill-sets"
import { chosenSetup } from "@/domain/set-up"

describe("what a skill set holds", () => {
  it("counts its skills, and names its tools when it has some", () => {
    expect(holds({ Skills: 1, Servers: [] })).toBe("1 skill")
    expect(holds({ Skills: 0, Servers: [] })).toBe("0 skills")
    expect(holds({ Skills: 2, Servers: ["memory", "search"] })).toBe(
      "2 skills · tools: memory, search"
    )
  })

  it("names its MCP servers, or none, and who changes them", () => {
    expect(serversLine(["memory"])).toBe(
      "memory An administrator changes these."
    )
    expect(serversLine([])).toBe("None chosen. An administrator changes these.")
  })
})

describe("the skill set an agent starts with", () => {
  const setups = [{ value: "reviewer" }, { value: "health" }]
  it("is the one the link named when the agent type can take it", () => {
    expect(chosenSetup(setups, "health")).toBe("health")
  })

  it("is none when the link named one it cannot take, or none", () => {
    expect(chosenSetup(setups, "other")).toBe("")
    expect(chosenSetup(setups, "")).toBe("")
  })
})
