import { describe, expect, it } from "vitest"

import { cn } from "@/lib/utils"

// A component's classes and the classes a page gives it are merged by cn, so a page's class replaces the component's class for the
// same property, and the result never depends on which rule comes later in the stylesheet.
describe("cn", () => {
  it("keeps the page's class over the component's for the same property", () => {
    expect(cn("px-4 text-muted-foreground", "text-foreground")).toBe(
      "px-4 text-foreground"
    )
    expect(cn("h-9 px-2.5", "h-8")).toBe("px-2.5 h-8")
  })

  it("keeps classes for different properties, and the variants of one property apart", () => {
    expect(cn("px-4", "py-3")).toBe("px-4 py-3")
    expect(cn("hidden", "@max-xl:hidden")).toBe("hidden @max-xl:hidden")
  })

  it("leaves out what a condition turns off", () => {
    expect(cn("text-right", false, undefined, "")).toBe("text-right")
  })
})
