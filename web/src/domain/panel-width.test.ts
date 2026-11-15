import { describe, expect, it } from "vitest"

import { clampWidth, keptWidth } from "@/domain/panel-width"

describe("the side panel's width", () => {
  it("stays between its bounds and within 40% of the window", () => {
    expect(clampWidth(10, 100)).toBe(16)
    expect(clampWidth(40, 100)).toBe(28)
    expect(clampWidth(26, 50)).toBe(20)
    expect(clampWidth(21.13, 100)).toBe(21.25)
  })

  it("falls back to the usual width when nothing usable was kept", () => {
    expect(keptWidth(null)).toBe(20)
    expect(keptWidth("junk")).toBe(20)
    expect(keptWidth("24rem")).toBe(24)
  })
})
