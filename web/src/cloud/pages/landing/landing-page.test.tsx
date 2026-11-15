import { fireEvent, render, screen, within } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { cloudPage } from "@/cloud/fixture"
import { LandingPage } from "@/cloud/pages/landing/landing-page"

const demoOffered = cloudPage("landing", {
  DemoPublic: true,
  DemoLeftLong: "30-minute",
  Book: "https://cal.example/gusto",
  Shot: "/static/hero-light.webp?v=1",
  ShotDark: "/static/hero-dark.webp?v=1",
})

describe("the landing page", () => {
  it("says what gusto is and has every section a visitor reads", () => {
    render(<LandingPage page={demoOffered} />)
    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "One workspace for every AI agent your team uses.",
      })
    ).toBeTruthy()
    for (const section of [
      "Choose how to run gusto",
      "Start in two steps",
      "What you get from day one",
      "Works with your agents, models and computers",
      "Tested across multiple agents.",
      "Simple pricing",
      "Help and support",
      "Try gusto with your team.",
    ]) {
      expect(
        screen.getByRole("heading", { level: 2, name: section })
      ).toBeTruthy()
    }
    expect(screen.getByText(/A person a month\./)).toBeTruthy()
    expect(
      screen.getByAltText(
        "Two people and their agents working in one room in gusto"
      )
    ).toBeTruthy()
  })

  it("leads to a demo and a call where anybody may start one", () => {
    render(<LandingPage page={demoOffered} />)
    const starts = screen.getAllByTestId("start")
    expect(starts.every((a) => a.getAttribute("href") === "/demo")).toBe(true)
    expect(starts[0].textContent).toBe("Try it now")
    expect(screen.getAllByTestId("book")[0].getAttribute("href")).toBe(
      "https://cal.example/gusto"
    )
    expect(screen.getByTestId("offer-terms").textContent).toMatch(
      /30-minute isolated demo/
    )
    expect(screen.getByTestId("sign-in").getAttribute("href")).toBe("/signin")
  })

  it("leads to sign-up with the free credit where only the team gives demos", () => {
    render(
      <LandingPage
        page={{
          ...demoOffered,
          DemoPublic: false,
          StartingCredit: 5,
          Book: "",
        }}
      />
    )
    expect(screen.getAllByTestId("start")[0].getAttribute("href")).toBe(
      "/signup"
    )
    expect(screen.getAllByTestId("start")[0].textContent).toBe("Start free")
    expect(screen.queryByTestId("book")).toBeNull()
    expect(screen.getByTestId("offer-terms").textContent).toMatch(
      /^Free to start, with \$5/
    )
  })

  it("offers a signed-in visitor the way into gusto instead of a sign-in", () => {
    render(<LandingPage page={{ ...demoOffered, SignedIn: true }} />)
    expect(screen.getByTestId("open-gusto").getAttribute("href")).toBe("/app")
    expect(screen.queryByTestId("sign-in")).toBeNull()
  })

  it("shows the operator's video in place of the picture", () => {
    const { container } = render(
      <LandingPage
        page={{
          ...demoOffered,
          Video: "https://cdn.example/demo.mp4",
          VideoFile: true,
        }}
      />
    )
    expect(container.querySelector("video")?.getAttribute("src")).toBe(
      "https://cdn.example/demo.mp4"
    )
    expect(container.querySelector("picture img[width='1568']")).toBeNull()
  })

  it("narrows what gusto works with to agents, models or places", () => {
    render(<LandingPage page={demoOffered} />)
    const list = screen.getByTestId("works-with")
    expect(within(list).getAllByRole("listitem")).toHaveLength(11)
    fireEvent.click(screen.getByRole("button", { name: "Models" }))
    expect(
      within(list)
        .getAllByRole("listitem")
        .map((li) => li.textContent)
    ).toEqual(["Your own model keyModel", "gusto’s modelsModel"])
  })

  it("copies the install command", async () => {
    const writeText = vi.fn(() => Promise.resolve())
    Object.assign(navigator, { clipboard: { writeText } })
    render(<LandingPage page={demoOffered} />)
    fireEvent.click(screen.getByTestId("copy-install"))
    expect(writeText).toHaveBeenCalledWith(
      screen.getByTestId("install-command").textContent
    )
    expect(await screen.findByText("Copied")).toBeTruthy()
  })
})
