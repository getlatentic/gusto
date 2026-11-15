import { screen, waitFor } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { setup } from "@/pages/setups/fixture"
import { SetupPage } from "@/pages/setups/setup-page"
import { SetupsPage } from "@/pages/setups/setups-page"
import { renderPage } from "@/test/render"

describe("the skill sets list", () => {
  it("links each skill set, saying what it holds", async () => {
    renderPage("/setups", "/setups", <SetupsPage />, {
      "/setups": {
        Setups: [{ Name: "reviewer", Skills: 2, Servers: ["memory"] }],
        Keeps: true,
        Refused: "",
      },
    })
    const link = await screen.findByTestId("setup-link")
    expect(link.getAttribute("href")).toBe("/setups/reviewer")
    expect(screen.getByText("2 skills · tools: memory")).toBeTruthy()
    expect(
      screen.getByText("Instructions and tools you can give any agent.")
    ).toBeTruthy()
    expect(screen.getByText("New agent").getAttribute("href")).toBe("/presets")
  })

  it("says where skill sets come from when there are none", async () => {
    renderPage("/setups", "/setups", <SetupsPage />, {
      "/setups": { Setups: [], Keeps: true, Refused: "" },
    })
    expect(await screen.findByText("No skill sets yet")).toBeTruthy()
    expect(
      screen.getByText("Admins add them in Admin, under Apps & skills.")
    ).toBeTruthy()
  })

  it("says nothing of skill sets on a server that keeps none", async () => {
    renderPage("/setups", "/setups", <SetupsPage />, {
      "/setups": { Setups: [], Keeps: false, Refused: "" },
    })
    await waitFor(() =>
      expect(document.querySelector('[aria-busy="false"]')).toBeTruthy()
    )
    expect(screen.queryByText("No skill sets yet")).toBeNull()
  })
})

describe("a skill set's page", () => {
  const draw = (page = setup) =>
    renderPage("/setups/reviewer", "/setups/:name", <SetupPage />, {
      "/setups/reviewer": page,
    })

  it("shows its skills, its servers and its integrations, and carries no control to change them", async () => {
    draw()
    expect(await screen.findByText("Use when asked to review.")).toBeTruthy()
    expect(screen.getByRole("heading", { name: "reviewer" })).toBeTruthy()
    expect(
      screen.getByText("memory An administrator changes these.")
    ).toBeTruthy()
    expect(screen.getByText("linear: 2 skills, 1 server")).toBeTruthy()
    expect(document.querySelector("form")).toBeNull()
    expect(screen.queryByText("Add a skill")).toBeNull()
    expect(screen.queryByText("Remove")).toBeNull()
    expect(document.querySelector('[action*="/drop"]')).toBeNull()
  })

  it("starts an agent with it, and says why an agent type cannot take it", async () => {
    draw()
    const start = await screen.findByTestId("start-with")
    expect(start.getAttribute("href")).toBe(
      "/instances/new?harness=opencode&setup=reviewer"
    )
    const unfit = screen.getByText("pi")
    expect(unfit.tagName).toBe("SPAN")
    expect(unfit.getAttribute("title")).toBe("it takes no MCP servers")
  })

  it("says why there are no servers where the deployment offers none", async () => {
    draw({ ...setup, Offers: false, Servers: [], Uses: [] })
    expect(
      await screen.findByText(/This deployment offers no MCP servers/)
    ).toBeTruthy()
    expect(screen.queryByText("Integrations")).toBeNull()
  })

  it("says what a skill set without skills is for", async () => {
    draw({ ...setup, Set: { ...setup.Set, Skills: [] } })
    expect(
      await screen.findByText(
        "No skills yet. A skill is instructions an agent reads when its description matches what it was asked."
      )
    ).toBeTruthy()
  })
})
