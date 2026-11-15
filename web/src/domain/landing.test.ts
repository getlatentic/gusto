import { describe, expect, it } from "vitest"

import { bookLabel, offerTerms, startHref, startLabel } from "@/domain/landing"

const demo = { DemoPublic: true, DemoLeftLong: "30-minute", StartingCredit: 5 }
const signUp = { ...demo, DemoPublic: false }

describe("the landing page's offer", () => {
  it("leads to a demo where anybody may start one", () => {
    expect(startHref(demo)).toBe("/demo")
    expect(startLabel(demo)).toBe("Try it now")
    expect(bookLabel(demo)).toBe("Talk to us")
    expect(offerTerms(demo)).toBe(
      "Free. No credit card. A 30-minute isolated demo, deleted when it ends."
    )
  })

  it("leads to sign-up, with the free credit, where only the team gives demos", () => {
    expect(startHref(signUp)).toBe("/signup")
    expect(startLabel(signUp)).toBe("Start free")
    expect(bookLabel(signUp)).toBe("Book a demo")
    expect(offerTerms(signUp)).toMatch(
      /^Free to start, with \$5 of gusto's models\./
    )
  })
})
