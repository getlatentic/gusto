import { screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { presets } from "@/pages/setups/fixture"
import { PresetsPage } from "@/pages/setups/presets-page"
import { renderPage } from "@/test/render"

const draw = (page = presets) =>
  renderPage("/presets", "/presets", <PresetsPage />, { "/presets": page })

describe("the presets page", () => {
  it("lists the person's presets, each to start a chat with or to remove", async () => {
    draw()
    const row = await screen.findByTestId("preset-row")
    expect(within(row).getByText("Reviewer")).toBeTruthy()
    expect(
      within(row).getByText("Software engineer on OpenCode with Stub Model")
    ).toBeTruthy()
    expect(within(row).getByText("New chat").getAttribute("href")).toBe(
      "/chat?preset=p1"
    )
    const remove = within(row).getByRole("button", { name: "Remove Reviewer" })
    expect(remove.getAttribute("title")).toBe("Remove")
    const form = (remove as HTMLButtonElement).form!
    expect(form.method).toBe("post")
    expect(form.getAttribute("action")).toBe("/presets/p1/delete")
  })

  it("has no list of the person's presets when they saved none", async () => {
    draw({ ...presets, Presets: [] })
    await screen.findByText("Built-in presets")
    expect(screen.queryByText("Your presets")).toBeNull()
  })

  it("shows the first built-in preset, what it can do and what to try asking, and the ways to use it", async () => {
    draw()
    const detail = await screen.findByTestId("role-detail")
    expect(within(detail).getByRole("heading", { level: 2 }).textContent).toBe(
      "Software engineer"
    )
    expect(within(detail).getByText("What it can do")).toBeTruthy()
    expect(
      within(detail).getByText("Finds the cause and fixes it.")
    ).toBeTruthy()
    expect(within(detail).getByText("Try asking")).toBeTruthy()
    expect(within(detail).getByText("Fix the failing test")).toBeTruthy()
    expect(within(detail).getByText("Details").getAttribute("href")).toBe(
      "/templates/engineering"
    )
    expect(
      within(detail).getByText("Use in a new chat").getAttribute("href")
    ).toBe("/chat?role=engineering")
    expect(
      screen.getByText("Build your own skill set").getAttribute("href")
    ).toBe("/setups")
  })

  it("shows the preset picked from the list", async () => {
    draw()
    const health = (await screen.findAllByTestId("pick-role"))[1]
    await userEvent.click(health)
    expect(health.getAttribute("aria-pressed")).toBe("true")
    expect(
      within(screen.getByTestId("role-detail")).getByText(
        "Draft a SOAP note from the visit notes"
      )
    ).toBeTruthy()
  })

  it("narrows the list by field of work and by a search, and says when nothing matches", async () => {
    draw()
    await screen.findByTestId("role-detail")
    const fields = screen.getByRole("group", { name: "Field of work" })
    await userEvent.click(within(fields).getByText("Health"))
    expect(screen.getAllByTestId("pick-role")).toHaveLength(1)
    expect(
      within(screen.getByTestId("role-detail")).getByRole("heading", {
        level: 2,
      }).textContent
    ).toBe("Clinical scribe")
    await userEvent.click(within(fields).getByText("All"))
    await userEvent.type(screen.getByTestId("role-search"), "nothing like it")
    expect(screen.queryAllByTestId("pick-role")).toHaveLength(0)
    expect(screen.getByText("No role matches.")).toBeTruthy()
  })

  it("says why a preset cannot be used here, and does not link it", async () => {
    draw()
    await userEvent.click((await screen.findAllByTestId("pick-role"))[2])
    const detail = screen.getByTestId("role-detail")
    const use = within(detail).getByText("Use in a new chat")
    expect(use.getAttribute("href")).toBeNull()
    expect(use.getAttribute("aria-disabled")).toBe("true")
    expect(
      within(detail).getByText(
        "This work only goes to models in your own network, and this deployment has none."
      )
    ).toBeTruthy()
  })

  it("names every control, offers a narrow window a menu of the presets, and never says harness", async () => {
    draw()
    await screen.findByTestId("role-detail")
    for (const button of screen.getAllByRole("button")) {
      expect(
        button.textContent || button.getAttribute("aria-label")
      ).toBeTruthy()
    }
    expect(screen.getByLabelText("Search presets")).toBeTruthy()
    expect(screen.getByLabelText("Built-in preset")).toBeTruthy()
    expect(document.body.textContent?.toLowerCase()).not.toContain("harness")
  })
})
