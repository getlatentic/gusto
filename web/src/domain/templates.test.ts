import { describe, expect, it } from "vitest"

import { canStart, choosesAgent, keptOut } from "@/domain/templates"

const open = { Restricted: false }
const restricted = { Restricted: true }

describe("starting a template", () => {
  it("keeps restricted work out only where no model is in the building", () => {
    expect(keptOut(restricted, false)).toBe(true)
    expect(keptOut(restricted, true)).toBe(false)
    expect(keptOut(open, false)).toBe(false)
  })

  it("starts when an agent here can take it and it is not kept out", () => {
    expect(canStart(open, false, 1)).toBe(true)
    expect(canStart(open, false, 0)).toBe(false)
    expect(canStart(restricted, false, 2)).toBe(false)
    expect(canStart(restricted, true, 1)).toBe(true)
  })

  it("offers a choice of agent only when more than one can take it", () => {
    expect(choosesAgent(open, false, 1)).toBe(false)
    expect(choosesAgent(open, false, 2)).toBe(true)
    expect(choosesAgent(restricted, false, 2)).toBe(false)
    expect(choosesAgent(restricted, true, 2)).toBe(true)
  })
})
