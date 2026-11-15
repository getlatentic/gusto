import { describe, expect, it } from "vitest"

import {
  offersSharedLink,
  providerWay,
  signInAction,
  type SignIn,
} from "@/domain/sign-in"

const page = (given: Partial<SignIn>): SignIn => ({
  Wrong: false,
  Local: false,
  Identity: false,
  Provider: false,
  Desktop: false,
  Shared: false,
  Refused: "",
  ...given,
})

describe("the sign-in page", () => {
  it("offers the provider on a page, and a browser to continue in in the desktop app", () => {
    expect(providerWay(page({ Identity: true, Provider: true }))).toBe(
      "provider"
    )
    expect(
      providerWay(page({ Desktop: true, Identity: true, Provider: true }))
    ).toBe("browser")
    expect(providerWay(page({ Desktop: true, Provider: true }))).toBe("browser")
    expect(providerWay(page({ Desktop: true }))).toBe("none")
    expect(providerWay(page({}))).toBe("none")
  })

  it("links to the shared password when its form is closed, except in a desktop app with no provider of its own", () => {
    expect(offersSharedLink(page({ Identity: true, Provider: true }))).toBe(
      true
    )
    expect(
      offersSharedLink(page({ Identity: true, Provider: true, Shared: true }))
    ).toBe(false)
    expect(offersSharedLink(page({ Desktop: true, Provider: true }))).toBe(
      false
    )
    expect(
      offersSharedLink(page({ Desktop: true, Identity: true, Provider: true }))
    ).toBe(true)
  })

  it("posts the desktop app's shared password so that it goes back to the app", () => {
    expect(signInAction(page({}))).toBe("/login")
    expect(signInAction(page({ Desktop: true }))).toBe("/login?desktop=1")
  })
})
