import { describe, expect, it } from "vitest"

import {
  clampWidth,
  filePanel,
  keptWidth,
  sidePanel,
} from "@/domain/panel-width"

describe("a panel's width", () => {
  it("stays between its bounds and within its share of the window", () => {
    expect(clampWidth(10, 100, sidePanel)).toBe(16)
    expect(clampWidth(40, 100, sidePanel)).toBe(28)
    expect(clampWidth(26, 50, sidePanel)).toBe(20)
    expect(clampWidth(21.13, 100, sidePanel)).toBe(21.25)
    expect(clampWidth(80, 100, filePanel)).toBe(60)
    expect(clampWidth(50, 60, filePanel)).toBe(42)
  })

  it("falls back to the usual width when nothing usable was kept", () => {
    expect(keptWidth(null, sidePanel)).toBe(20)
    expect(keptWidth("junk", filePanel)).toBe(36)
    expect(keptWidth("24rem", sidePanel)).toBe(24)
  })
})
