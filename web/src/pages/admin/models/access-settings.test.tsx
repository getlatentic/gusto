import { fireEvent, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { drawModels, fieldsOf, formOf, modelsData } from "./test-data"
import type { ModelsData } from "./types"

const access = "/admin/models?tab=access"
const settings = "/admin/models?tab=settings"

function settingsData(change: (page: ModelsData) => void = () => {}) {
  return modelsData("settings", (page) => {
    page.Defaults = [
      { Value: "claude|platform/qwen", Label: "Claude · Qwen", Chosen: false },
      { Value: "codex|acme/big-1", Label: "Codex · big-1", Chosen: true },
    ]
    page.DefaultNow = "Codex · big-1"
    page.RunTokens = { Bound: true, Err: "" }
    page.Steps = [
      {
        Version: 3,
        At: 0,
        Then: ["retired model acme/big", "added model acme/small"],
      },
      { Version: 2, At: 0, Then: null },
    ]
    change(page)
  })
}

describe("who may use the models", () => {
  it("lists the people limited, each with a way to let them use everything", async () => {
    drawModels(
      modelsData("access", (page) => {
        page.Limits = [
          { email: "bob@acme.com", models: ["platform/qwen", "acme/big-1"] },
        ]
      }),
      access
    )
    const row = (await screen.findByText("bob@acme.com")).closest("tr")!
    expect(row.textContent).toContain("platform/qwen, acme/big-1")
    const form = formOf(
      within(row).getByRole("button", { name: "Let use everything" })
    )
    expect(form.getAttribute("action")).toBe("/admin/models/access/clear")
    expect(fieldsOf(form)).toBe("email=bob%40acme.com")
  })

  it("limits a person to the models ticked, offering the people there are", async () => {
    drawModels(
      modelsData(
        "access",
        (page) => (page.People = ["ada@acme.com", "bob@acme.com"])
      ),
      access
    )
    expect(
      await screen.findByText(
        "Nobody is limited, so everybody may use every model."
      )
    ).toBeTruthy()
    fireEvent.click(screen.getByRole("button", { name: "Limit a person" }))
    const dialog = await screen.findByRole("dialog")
    const form = dialog.querySelector("form")!
    expect(form.getAttribute("action")).toBe("/admin/models/access")
    const person = within(dialog).getByLabelText("Person")
    const offered = document.getElementById(person.getAttribute("list")!)!
    expect([...offered.querySelectorAll("option")].map((o) => o.value)).toEqual(
      ["ada@acme.com", "bob@acme.com"]
    )
    fireEvent.change(person, { target: { value: "ada@acme.com" } })
    fireEvent.click(
      within(dialog).getByRole("checkbox", { name: "acme/big-1" })
    )
    expect(fieldsOf(form)).toBe("email=ada%40acme.com&model=acme%2Fbig-1")
  })

  it("offers no limit when there are no models to limit a person to", async () => {
    drawModels(
      modelsData("access", (page) => (page.View.Available = null)),
      access
    )
    await screen.findByText(/Everybody may use every model unless/)
    expect(screen.queryByRole("button", { name: "Limit a person" })).toBeNull()
  })
})

describe("the Settings tab", () => {
  it("says what a new chat starts on, and changes it in a dialog holding the choice", async () => {
    drawModels(settingsData(), settings)
    expect(
      await screen.findByText(
        "Codex · big-1. A person's own last choice comes first."
      )
    ).toBeTruthy()
    fireEvent.click(screen.getByRole("button", { name: "Change" }))
    const dialog = await screen.findByRole("dialog")
    const form = dialog.querySelector("form")!
    expect(form.getAttribute("action")).toBe("/admin/chat-default")
    expect(fieldsOf(form)).toBe("default=codex%7Cacme%2Fbig-1")
  })

  it("starts a new chat on the first that can run until one is chosen", async () => {
    drawModels(
      settingsData((page) => {
        page.Defaults = page.Defaults!.map((each) => ({
          ...each,
          Chosen: false,
        }))
      }),
      settings
    )
    fireEvent.click(await screen.findByRole("button", { name: "Change" }))
    const dialog = await screen.findByRole("dialog")
    expect(fieldsOf(dialog.querySelector("form")!)).toBe("default=")
    expect(within(dialog).getByText("The first that can run")).toBeTruthy()
  })

  it("chooses how sandboxes reach a model", async () => {
    drawModels(settingsData(), settings)
    const plain = await screen.findByRole("button", {
      name: "Use plain tokens",
    })
    expect(formOf(plain).getAttribute("action")).toBe("/admin/run-tokens")
    expect(fieldsOf(formOf(plain))).toBe("bound=no")
    expect(screen.getByText(/bound to its sandbox/)).toBeTruthy()
  })

  it("offers to bind runs again, and says when the setting could not be read", async () => {
    drawModels(
      settingsData(
        (page) =>
          (page.RunTokens = { Bound: false, Err: "settings.json is damaged" })
      ),
      settings
    )
    const bind = await screen.findByRole("button", {
      name: "Bind to the sandbox",
    })
    expect(fieldsOf(formOf(bind))).toBe("bound=yes")
    expect(
      screen.getByText(
        "The setting could not be read, so runs are bound. settings.json is damaged"
      )
    ).toBeTruthy()
  })

  it("goes back to an earlier version, saying what it undoes", async () => {
    drawModels(settingsData(), settings)
    const three = (await screen.findByText("Version 3")).closest("tr")!
    expect(three.textContent).toContain(
      "then: retired model acme/big; added model acme/small"
    )
    const form = formOf(
      within(three).getByRole("button", { name: "Go back to it" })
    )
    expect(form.getAttribute("action")).toBe("/admin/models/restore")
    expect(fieldsOf(form)).toBe("version=3")
    const two = screen.getByText("Version 2").closest("tr")!
    expect(two.textContent).toContain("then: no change you can see")
  })

  it("has only the earlier versions where the organization has no settings", async () => {
    drawModels(
      settingsData((page) => {
        page.Defaults = undefined
        page.RunTokens = undefined
      }),
      settings
    )
    await screen.findByText("Go back to an earlier version")
    expect(screen.queryByText("A new chat starts on")).toBeNull()
    expect(screen.queryByText(/how sandboxes reach a model/)).toBeNull()
  })
})
