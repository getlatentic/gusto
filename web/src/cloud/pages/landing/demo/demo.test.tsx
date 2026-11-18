import { fireEvent, render, screen, within } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { ProductDemo } from "@/cloud/pages/landing/demo/product-demo"
import { roomScene } from "@/cloud/pages/landing/demo/room-scene"
import { frameAt, type Scene } from "@/cloud/pages/landing/demo/script"
import {
  clockAfter,
  startClock,
} from "@/cloud/pages/landing/demo/use-demo-clock"

const scene: Scene<{ draft: string; sent: boolean }> = {
  length: 3000,
  start: { draft: "", sent: false },
  steps: [
    { at: 0, cursor: "draft", label: "Type" },
    { at: 100, type: { key: "draft", text: "hello", perChar: 100 } },
    { at: 1000, cursor: "send", label: "Send" },
    { at: 1500, click: true, set: { draft: "", sent: true } },
  ],
}

describe("a demo scene's script", () => {
  it("shows what has happened by a moment, typing a character at a time", () => {
    expect(frameAt(scene, 0)).toEqual({
      state: { draft: "", sent: false },
      cursor: "draft",
      label: "Type",
      clicking: false,
    })
    expect(frameAt(scene, 400).state.draft).toBe("hel")
    expect(frameAt(scene, 900).state.draft).toBe("hello")
    expect(frameAt(scene, 1100)).toMatchObject({
      cursor: "send",
      label: "Send",
    })
  })

  it("rings a click briefly, and keeps what the click changed", () => {
    expect(frameAt(scene, 1600)).toMatchObject({
      clicking: true,
      state: { draft: "", sent: true },
    })
    expect(frameAt(scene, 2500).clicking).toBe(false)
    expect(frameAt(scene, Infinity).state).toEqual({ draft: "", sent: true })
  })

  it("ends the room scene with the plan written and the review asked for", () => {
    expect(frameAt(roomScene, Infinity).state).toMatchObject({
      sent: true,
      replied: true,
      followUp: true,
    })
  })
})

describe("the demo's clock", () => {
  const lengths = [100, 100]

  it("plays every scene in turn and starts again", () => {
    let clock = startClock
    for (let at = 0; at < 2; at++)
      clock = clockAfter(clock, { kind: "tick", lengths })
    expect(clock).toMatchObject({ scene: 1, ms: 0 })
    for (let at = 0; at < 2; at++)
      clock = clockAfter(clock, { kind: "tick", lengths })
    expect(clock).toMatchObject({ scene: 0, ms: 0 })
  })

  it("holds a scene a person chose at its end, and pauses when asked", () => {
    let clock = clockAfter(startClock, { kind: "choose", scene: 1 })
    for (let at = 0; at < 5; at++)
      clock = clockAfter(clock, { kind: "tick", lengths })
    expect(clock).toMatchObject({ scene: 1, ms: 100 })
    expect(clockAfter(clock, { kind: "pause" }).paused).toBe(true)
  })
})

function motion(calm: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: calm && query.includes("no-preference"),
    media: query,
    addEventListener() {},
    removeEventListener() {},
  }))
}

afterEach(() => vi.unstubAllGlobals())

describe("the product demo", () => {
  it("has a tab for each scene and a panel that says in words what it shows", () => {
    motion(true)
    render(<ProductDemo />)
    const tabs = within(screen.getByRole("tablist")).getAllByRole("tab")
    expect(tabs.map((tab) => tab.getAttribute("aria-label"))).toEqual([
      "Shared rooms",
      "Private computers",
      "Stay in control",
    ])
    expect(tabs[0].getAttribute("aria-selected")).toBe("true")
    const panel = screen.getByRole("tabpanel")
    expect(panel.getAttribute("aria-labelledby")).toBe(tabs[0].id)
    expect(panel.textContent).toMatch(/^In a shared room, Sam asks/)
    fireEvent.click(tabs[2])
    expect(tabs[2].getAttribute("aria-selected")).toBe("true")
    expect(screen.getByRole("tabpanel").textContent).toMatch(
      /^An admin raises Priya's budget/
    )
    expect(screen.getByRole("button", { name: "Pause the demo" })).toBeTruthy()
  })

  it("shows each scene as it ends, with no cursor, to a person who asked for less motion", () => {
    motion(false)
    render(<ProductDemo />)
    expect(screen.getByRole("tabpanel").textContent).toContain(
      "release-plan.md"
    )
    expect(screen.queryByTestId("demo-cursor")).toBeNull()
    expect(screen.queryByRole("button", { name: "Pause the demo" })).toBeNull()
  })
})
