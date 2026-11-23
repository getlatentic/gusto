import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { BlankPage } from "@/pages/setups/blank-page"
import { blank, setUp } from "@/pages/setups/fixture"
import { SetUpPage } from "@/pages/setups/set-up-page"
import { renderPage } from "@/test/render"

const field = (form: HTMLFormElement, name: string) =>
  form.querySelector(`[name="${name}"]`) as HTMLInputElement | null

describe("starting an agent of one's own", () => {
  it("posts a name, an agent type, a skill set and a repository to start it", async () => {
    renderPage("/new/blank", "/new/blank", <BlankPage />, {
      "/new/blank": blank,
    })
    const form = (await screen.findByTestId("blank-form")) as HTMLFormElement
    expect(form.method).toBe("post")
    expect(form.getAttribute("action")).toBe("/instances")
    const title = screen.getByLabelText("Name it") as HTMLInputElement
    expect([title.name, title.placeholder, title.maxLength]).toEqual([
      "title",
      "Agent",
      60,
    ])
    expect(screen.getByText("Agent type")).toBeTruthy()
    expect(screen.getByText("OpenCode")).toBeTruthy()
    expect(screen.getByText("Anthropic's agent.")).toBeTruthy()
    expect(field(form, "harness")?.value).toBe("opencode")
    expect(field(form, "setup")?.value).toBe("")
    expect(field(form, "repo")?.placeholder).toBe("git@github.com:you/repo.git")
    expect(
      screen.getByText("How each agent type did").getAttribute("href")
    ).toBe("/engines")
    expect(screen.getByText("New agent").getAttribute("href")).toBe("/presets")
    expect(document.body.textContent?.toLowerCase()).not.toContain("harness")
  })

  it("offers no skill set where there is none, and says when nothing can start", async () => {
    renderPage("/new/blank", "/new/blank", <BlankPage />, {
      "/new/blank": { ...blank, Engines: [], Setups: [] },
    })
    expect(await screen.findByText("Nothing can start here yet")).toBeTruthy()
    expect(
      screen.getByText("Ask whoever runs this to add an agent.")
    ).toBeTruthy()
    expect(document.querySelector("form")).toBeNull()
  })
})

describe("setting up an agent type", () => {
  const draw = (page = setUp) =>
    renderPage(
      "/instances/new?harness=opencode",
      "/instances/new",
      <SetUpPage />,
      { "/instances/new?harness=opencode": page }
    )

  it("posts the agent type with a model, a repository and the skill set the link named", async () => {
    draw()
    const form = (await screen.findByTestId("set-up-form")) as HTMLFormElement
    expect(form.getAttribute("action")).toBe("/instances")
    expect(form.method).toBe("post")
    expect(field(form, "harness")?.value).toBe("opencode")
    expect(field(form, "then")).toBeNull()
    expect(field(form, "model")?.value).toBe("stub/stub-model")
    expect(field(form, "setup")?.value).toBe("health")
    expect(screen.getByText("Stub Model")).toBeTruthy()
    expect(
      screen.getByText("optional; without one it starts in an empty folder")
    ).toBeTruthy()
    expect(screen.getByText("skills and MCP servers to give it")).toBeTruthy()
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe(
      "OpenCode"
    )
    expect(document.body.textContent?.toLowerCase()).not.toContain("harness")
  })

  it("remembers that a task was asked for, and takes a model by name where none is offered", async () => {
    draw({ ...setUp, Then: "task", Models: [], Setups: [] })
    const form = (await screen.findByTestId("set-up-form")) as HTMLFormElement
    expect(field(form, "then")?.value).toBe("task")
    expect(field(form, "model")?.placeholder).toBe(
      "the model this deployment reaches"
    )
    expect(field(form, "setup")).toBeNull()
  })
})
