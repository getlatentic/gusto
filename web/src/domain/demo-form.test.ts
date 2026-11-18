import { describe, expect, it } from "vitest"

import { demoFormWords } from "@/domain/demo-form"

describe("the demo form's words", () => {
  it("speak to the team where only the team gives demos, and offer no call", () => {
    const words = demoFormWords({
      ForTeam: true,
      WorkOnly: true,
      DemoLeft: "30 minutes",
    })
    expect(words.title).toBe("Start a demo")
    expect(words.intro).toContain("deleted after 30 minutes")
    expect(words.company).toBe("Their company")
    expect(words.offersCall).toBe(false)
  })

  it("speak to a visitor otherwise, and ask for a work email only where only one may sign up", () => {
    const words = demoFormWords({
      ForTeam: false,
      WorkOnly: false,
      DemoLeft: "30 minutes",
    })
    expect(words.title).toBe("Try gusto free for 30 minutes")
    expect(words.email).toBe("Email")
    expect(words.offersCall).toBe(true)
    expect(
      demoFormWords({ ForTeam: false, WorkOnly: true, DemoLeft: "30 minutes" })
        .email
    ).toBe("Work email")
  })
})
