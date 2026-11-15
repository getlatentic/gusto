import { fireEvent, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { drawModels, fieldsOf, modelsData } from "./test-data"
import type { ModelsData } from "./types"

const sources = [
  { Kind: "file", Words: "a file on this machine", On: true },
  { Kind: "ssm", Words: "an AWS Parameter Store parameter", On: false },
  { Kind: "secretsmanager", Words: "an AWS Secrets Manager secret", On: false },
]

function providers(change: (page: ModelsData) => void = () => {}) {
  return modelsData("providers", (page) => {
    page.Sources = structuredClone(sources)
    page.Providers = [
      {
        Name: "platform",
        Href: "/admin/models?tab=providers&provider=platform",
        Address: "https://models.operator.example",
        Key: "the operator's",
        State: "on",
        More: 0,
        Models: 1,
        Operator: true,
      },
      {
        Name: "acme",
        Href: "/admin/models?tab=providers&provider=acme",
        Address: "https://api.theirs.example",
        Key: "kept here",
        State: "off",
        More: 1,
        Models: 2,
        Operator: false,
      },
    ]
    change(page)
  })
}

describe("the Providers tab", () => {
  it("lists each provider, which leads to its own page", async () => {
    drawModels(providers(), "/admin/models?tab=providers")
    const acme = await screen.findByRole("link", { name: "acme" })
    expect(acme.getAttribute("href")).toBe(
      "/admin/models?tab=providers&provider=acme"
    )
    const row = acme.closest("tr")!
    expect(row.textContent).toContain("and 1 more")
    expect(row.textContent).toContain("off: waiting for you to turn it on")
    const platform = screen
      .getByRole("link", { name: "platform" })
      .closest("tr")!
    expect(platform.textContent).toContain("set by the operator")
    expect(platform.textContent).toContain("on")
  })

  it("says there are no providers yet", async () => {
    drawModels(
      providers((page) => (page.Providers = null)),
      "/admin/models?tab=providers"
    )
    expect(
      await screen.findByText(
        "No providers yet. Add a provider, then the models it answers."
      )
    ).toBeTruthy()
  })

  it("lets the admin of their own installation say what a provider may be", async () => {
    drawModels(
      providers((page) => (page.View.Editable = true)),
      "/admin/models?tab=providers"
    )
    expect(
      await screen.findByText(
        "A provider may not be at a private address, and its key is typed here or named as a file on this machine."
      )
    ).toBeTruthy()
    fireEvent.click(screen.getByRole("button", { name: "Change" }))
    const dialog = await screen.findByRole("dialog")
    const form = dialog.querySelector("form")!
    expect(form.getAttribute("action")).toBe("/admin/models/policy")
    expect(fieldsOf(form)).toBe("keys=file")
    fireEvent.click(
      within(dialog).getByRole("checkbox", { name: /private address/ })
    )
    expect(fieldsOf(form)).toBe("private=on&keys=file")
  })

  it("says the operator fixes what a provider may be on a deployment that is not the admin's", async () => {
    drawModels(
      providers((page) => (page.View.Private = true)),
      "/admin/models?tab=providers"
    )
    expect(
      await screen.findByText(/operator fixes where providers may be/)
    ).toBeTruthy()
    expect(screen.getByText(/Private addresses are allowed\./)).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Change" })).toBeNull()
  })
})
