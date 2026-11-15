import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, expect, it } from "vitest"

import { template } from "@/pages/setups/fixture"
import { TemplatePage } from "@/pages/setups/template-page"
import type { TemplateData } from "@/pages/setups/types"
import { renderPage } from "@/test/render"

const draw = (page: TemplateData = template) =>
  renderPage("/templates/health", "/templates/:name", <TemplatePage />, {
    "/templates/health": page,
  })

const restricted = { ...template.Template, Restricted: true }

describe("a template's page", () => {
  it("starts the template on the first agent that can take it", async () => {
    draw()
    const start = (await screen.findByTestId(
      "start-template"
    )) as HTMLButtonElement
    expect(start.textContent).toBe("Start Clinical scribe")
    const form = start.form!
    expect(form.method).toBe("post")
    expect(form.getAttribute("action")).toBe("/templates/health")
    expect(
      (form.querySelector('[name="harness"]') as HTMLInputElement).value
    ).toBe("opencode")
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(
      "Clinical scribe"
    )
    expect(screen.getByText("Presets").getAttribute("href")).toBe("/presets")
  })

  it("says what it can do and what to try asking", async () => {
    draw()
    expect(await screen.findByText("What it can do")).toBeTruthy()
    expect(screen.getByText("Writes the note.")).toBeTruthy()
    expect(screen.getByText("Try asking")).toBeTruthy()
    expect(
      screen.getByText("Draft a SOAP note from the visit notes")
    ).toBeTruthy()
  })

  it("folds away the instructions it follows, its sample files and the choice of agent", async () => {
    draw()
    await userEvent.click(await screen.findByText("Details"))
    expect(screen.getByText("Instructions it follows")).toBeTruthy()
    expect(screen.getByText("visit-notes.txt")).toBeTruthy()
    expect(screen.getByText("Invented for demonstration.")).toBeTruthy()
    const choose = screen.getByText("Start with this agent").closest("form")!
    expect(choose.getAttribute("action")).toBe("/templates/health")
    expect(
      (choose.querySelector('[name="harness"]') as HTMLInputElement).value
    ).toBe("opencode")
    await userEvent.click(screen.getByText("Read"))
    expect(screen.getByText("Start with the subjective.")).toBeTruthy()
  })

  it("does not start restricted work where no model is in the building, and says why", async () => {
    draw({ ...template, Template: restricted })
    expect(await screen.findByText("Stays inside your network")).toBeTruthy()
    expect(
      screen.getByText(
        "It needs a model in your own network, and this deployment has none. gusto will not send this work anywhere else."
      )
    ).toBeTruthy()
    const start = screen.getByRole("button", { name: "Start" })
    expect((start as HTMLButtonElement).disabled).toBe(true)
    expect(screen.queryByTestId("start-template")).toBeNull()
    await userEvent.click(screen.getByText("Details"))
    expect(screen.queryByText("Start with this agent")).toBeNull()
  })

  it("starts restricted work where a model is in the building, saying where it goes", async () => {
    draw({ ...template, Template: restricted, InBuilding: true })
    expect(
      await screen.findByText(
        "This work only goes to models in your own network."
      )
    ).toBeTruthy()
    expect(screen.getByTestId("start-template")).toBeTruthy()
  })

  it("cannot be started where no agent can take it", async () => {
    draw({ ...template, Harnesses: [] })
    const start = await screen.findByRole("button", { name: "Start" })
    expect((start as HTMLButtonElement).disabled).toBe(true)
    expect(document.body.textContent?.toLowerCase()).not.toContain("harness")
  })
})
