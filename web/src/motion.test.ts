import { describe, expect, it } from "vitest"

import styles from "@/index.css?raw"

describe("the app's stylesheet", () => {
  it("stops every transition and animation for a person who asks for less motion", () => {
    const rule = styles
      .replace(/\s+/g, " ")
      .match(
        /@media \(prefers-reduced-motion: reduce\) \{ \*, \*::before, \*::after \{([^}]*)\}/
      )
    expect(rule?.[1]).toContain("transition-duration: 0s !important")
    expect(rule?.[1]).toContain("animation-duration: 0s !important")
  })
})
