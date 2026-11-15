import { describe, expect, it } from "vitest"

import { bytes, shortWhen } from "@/lib/format"

describe("sizes and dates at a glance", () => {
  it("says a size in the unit that reads best", () => {
    expect(bytes(512)).toBe("512 B")
    expect(bytes(12_300)).toBe("12 KB")
    expect(bytes(3_500_000)).toBe("3.3 MB")
  })

  it("says a date only when it was not today, and a year only when it was not this one", () => {
    const now = new Date(2026, 10, 15, 12, 0)
    expect(shortWhen(0, now)).toBe("")
    expect(
      shortWhen(new Date(2026, 10, 15, 9, 5).getTime() / 1000, now)
    ).toMatch(/9:05/)
    expect(shortWhen(new Date(2026, 9, 3).getTime() / 1000, now)).not.toMatch(
      /2026/
    )
    expect(shortWhen(new Date(2025, 9, 3).getTime() / 1000, now)).toMatch(
      /2025/
    )
  })
})
