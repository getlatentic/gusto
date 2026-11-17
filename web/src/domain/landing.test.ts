import { describe, expect, it } from "vitest"

import { offerTerms } from "@/domain/landing"

describe("the landing page's offer", () => {
  it("says what a person gets free when they sign up", () => {
    expect(offerTerms(5)).toBe(
      "Free to start with $5 of model credit. No credit card."
    )
    expect(offerTerms(0)).toBe("Free to start. No credit card.")
  })
})
