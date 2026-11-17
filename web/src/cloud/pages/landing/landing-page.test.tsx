import { render, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { cloudPage } from "@/cloud/fixture"
import { LandingPage } from "@/cloud/pages/landing/landing-page"

const landing = cloudPage("landing", {
  Book: "https://cal.example/gusto",
})

describe("the landing page", () => {
  it("says what gusto is and has every section a visitor reads", () => {
    render(<LandingPage page={landing} />)
    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "Bring your team and AI agents together.",
      })
    ).toBeTruthy()
    for (const section of [
      "See gusto at work",
      "Your agents, your choice.",
      "Choose how to run gusto",
      "Try gusto with your team.",
    ]) {
      expect(
        screen.getByRole("heading", { level: 2, name: section })
      ).toBeTruthy()
    }
    expect(screen.getByText(/A person a month\./)).toBeTruthy()
  })

  it("leads to sign-up and to a call", () => {
    render(<LandingPage page={landing} />)
    const starts = screen.getAllByTestId("start")
    expect(starts.every((a) => a.getAttribute("href") === "/signup")).toBe(true)
    expect(starts.every((a) => a.textContent === "Get started")).toBe(true)
    expect(screen.getAllByTestId("book")[0].getAttribute("href")).toBe(
      "https://cal.example/gusto"
    )
    expect(screen.getByTestId("sign-in").getAttribute("href")).toBe("/signin")
  })

  it("offers the pilot guide in place of a call where there is nowhere to book one", () => {
    render(<LandingPage page={{ ...landing, Book: "" }} />)
    expect(screen.queryByTestId("book")).toBeNull()
    expect(
      screen.getByRole("link", { name: "Read the pilot guide" })
    ).toBeTruthy()
  })

  it("offers a signed-in visitor the way into gusto instead of a sign-in", () => {
    render(<LandingPage page={{ ...landing, SignedIn: true }} />)
    expect(screen.getByTestId("open-gusto").getAttribute("href")).toBe("/app")
    expect(screen.queryByTestId("sign-in")).toBeNull()
    const starts = screen.getAllByTestId("start")
    expect(starts.length).toBeGreaterThan(0)
    expect(starts.every((a) => a.getAttribute("href") === "/app")).toBe(true)
  })

  it("shows the product at work right under the promise", () => {
    render(<LandingPage page={landing} />)
    const main = document.querySelector("main")!
    expect(within(main).getByTestId("product-demo")).toBeTruthy()
    expect(main.querySelector("video, iframe")).toBeNull()
  })

  it("says what gusto is worth in the hero, and what it runs with right after", () => {
    render(<LandingPage page={landing} />)
    expect(document.querySelector("main")?.textContent).toContain(
      "with control over spending, models and integrations."
    )
    const features = document.getElementById("features")!
    expect(
      within(features)
        .getAllByRole("heading", { level: 3 })
        .map((h) => h.textContent)
    ).toEqual([
      "Collaborate in shared rooms",
      "Private cloud computers",
      "Stay in control",
    ])
    expect(
      within(screen.getByRole("list", { name: "Deploy where it fits" }))
        .getAllByRole("listitem")
        .map((li) => li.textContent)
    ).toEqual(["Hosted by gusto", "Your own cloud", "Self-hosted"])
    expect(
      within(screen.getByTestId("agents"))
        .getAllByRole("listitem")
        .map((li) => li.textContent)
    ).toEqual(["Claude Code", "Codex", "OpenCode", "Pi", "Hermes", "DeepSeek"])
  })
})
