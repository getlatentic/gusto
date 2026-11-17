import { render } from "@testing-library/react"
import { gsap } from "gsap"
import { ScrollTrigger } from "gsap/ScrollTrigger"
import { beforeAll, describe, expect, it, vi } from "vitest"

import { cloudPage } from "@/cloud/fixture"
import { LandingPage } from "@/cloud/pages/landing/landing-page"

const page = cloudPage("landing")

function motion(calm: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: calm && query.includes("no-preference"),
    media: query,
    addListener() {},
    removeListener() {},
    addEventListener() {},
    removeEventListener() {},
  }))
}

// at puts every element of the page this far down it.
function at(top: number) {
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockReturnValue({
    top,
  } as DOMRect)
}

// The page fetches its motion after it is drawn; fetching it once here keeps each test from waiting on the first fetch.
beforeAll(() => import("@/cloud/pages/landing/landing-motion"))

describe("the landing page's motion", () => {
  it("hides the titles and cards below the fold until they scroll into view", async () => {
    motion(true)
    at(innerHeight + 200)
    const hide = vi.spyOn(gsap, "set")
    render(<LandingPage page={page} />)
    await vi.waitFor(() => expect(hide).toHaveBeenCalled())
    const hidden = hide.mock.calls[0][0] as HTMLElement[]
    expect(hidden.length).toBeGreaterThan(10)
    expect(hidden.every((each) => each.hasAttribute("data-reveal"))).toBe(true)
  })

  it("leaves what is already on screen as it is", async () => {
    motion(true)
    at(0)
    const look = vi.spyOn(gsap.utils, "toArray")
    const batch = vi.spyOn(ScrollTrigger, "batch")
    render(<LandingPage page={page} />)
    await vi.waitFor(() => expect(look).toHaveBeenCalled())
    expect(batch).not.toHaveBeenCalled()
  })

  it("moves nothing for a person who asked for less motion", async () => {
    motion(false)
    at(innerHeight + 200)
    const hide = vi.spyOn(gsap, "set")
    const drawn = render(<LandingPage page={page} />)
    await new Promise((settled) => setTimeout(settled, 50))
    expect(hide).not.toHaveBeenCalled()
    for (const each of drawn.container.querySelectorAll<HTMLElement>(
      "[data-reveal]"
    ))
      expect(each.style.opacity).toBe("")
  })
})
