import { describe, expect, it } from "vitest"

import { currentStep, memberActions, showsWorkspaces } from "@/domain/cloud-org"

const admin = { IsAdmin: true, Admins: 1, Person: { Email: "ada@acme.com" } }

describe("what an organization's page offers", () => {
  it("offers a runner to be made an admin and removed", () => {
    expect(
      memberActions({ Email: "bob@acme.com", Role: "runner" }, admin)
    ).toEqual({ makeRunner: false, makeAdmin: true, remove: true })
  })

  it("does not offer the last admin to be made a runner, nor to remove themselves", () => {
    expect(
      memberActions({ Email: "ada@acme.com", Role: "admin" }, admin)
    ).toEqual({ makeRunner: false, makeAdmin: false, remove: false })
    expect(
      memberActions(
        { Email: "ada@acme.com", Role: "admin" },
        { ...admin, Admins: 2 }
      ).makeRunner
    ).toBe(true)
  })

  it("offers nothing to a runner", () => {
    const runner = { ...admin, IsAdmin: false }
    expect(
      memberActions({ Email: "bob@acme.com", Role: "runner" }, runner)
    ).toEqual({ makeRunner: false, makeAdmin: false, remove: false })
    expect(showsWorkspaces(runner, 1)).toBe(false)
    expect(showsWorkspaces(runner, 2)).toBe(true)
  })

  it("names the stage a demo is at while it is made ready", () => {
    expect(
      currentStep([
        { Label: "Getting a computer ready", Mark: "done" },
        { Label: "Setting up your demo", Mark: "now" },
      ])?.Label
    ).toBe("Setting up your demo")
    expect(currentStep(null)).toBeUndefined()
  })
})
