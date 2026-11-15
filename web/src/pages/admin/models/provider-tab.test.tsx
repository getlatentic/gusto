import { fireEvent, screen, within } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"

import { acme, drawModels, fieldsOf, formOf, modelsData } from "./test-data"
import type { ModelsData } from "./types"

const acmePage = "/admin/models?tab=providers&provider=acme"

function acmeData(change: (page: ModelsData) => void = () => {}) {
  return modelsData("providers", (page) => {
    page.Provider = structuredClone(acme)
    page.Models = [
      {
        Name: "big-1",
        Full: "acme/big-1",
        Price: { inputPerMillion: 0.5, outputPerMillion: 1.5 },
        Pending: true,
      },
    ]
    change(page)
  })
}

const posts = (name: string) =>
  formOf(screen.getByRole("button", { name })).getAttribute("action")

describe("one provider of the organization's", () => {
  it("is the page, under the way back to the providers, waiting to be turned on", async () => {
    drawModels(acmeData(), acmePage)
    expect(
      await screen.findByRole("heading", { level: 1, name: "acme" })
    ).toBeTruthy()
    expect(
      screen.getByRole("link", { name: "Models" }).getAttribute("href")
    ).toBe("/admin/models?tab=providers")
    expect(screen.getByText("off: waiting for you to turn it on")).toBeTruthy()
    expect(posts("Turn on acme")).toBe("/admin/models/providers/acme/enable")
    expect(posts("Retire acme")).toBe("/admin/models/providers/acme/retire")
    expect(posts("Find its models")).toBe(
      "/admin/models/providers/acme/discover"
    )
    expect(screen.queryByRole("button", { name: "Turn off acme" })).toBeNull()
  })

  it("can be turned off once it is on", async () => {
    drawModels(
      acmeData((page) => (page.Provider!.Pending = false)),
      acmePage
    )
    await screen.findByRole("heading", { level: 1, name: "acme" })
    expect(posts("Turn off acme")).toBe("/admin/models/providers/acme/disable")
  })

  it("says each address and its kind, under one key, and removes one added beside the first", async () => {
    drawModels(acmeData(), acmePage)
    expect(await screen.findByText("OpenAI-compatible")).toBeTruthy()
    expect(screen.getByText("Anthropic-compatible")).toBeTruthy()
    expect(screen.getByText(/the same at each address/)).toBeTruthy()
    expect(screen.getByText(/^Key: kept here/)).toBeTruthy()
    const removes = screen.getAllByRole("button", { name: /^Remove/ })
    expect(removes).toHaveLength(1)
    const form = formOf(removes[0])
    expect(form.getAttribute("action")).toBe(
      "/admin/models/providers/acme/addresses/remove"
    )
    expect(fieldsOf(form)).toBe(
      "address=https%3A%2F%2Fapi.theirs.example%2Fanthropic"
    )
  })

  it("says why it is not in use", async () => {
    drawModels(
      acmeData((page) => (page.Provider!.Problem = "its key is missing")),
      acmePage
    )
    expect(
      await screen.findByText("Not in use: its key is missing")
    ).toBeTruthy()
  })

  it("adds a model with its prices, and another address of a kind", async () => {
    drawModels(acmeData(), acmePage)
    fireEvent.click(await screen.findByRole("button", { name: "Add model" }))
    let dialog = await screen.findByRole("dialog")
    let form = dialog.querySelector("form")!
    expect(form.getAttribute("action")).toBe(
      "/admin/models/providers/acme/models"
    )
    expect(
      within(dialog).getByLabelText("Model, as acme names it")
    ).toBeTruthy()
    expect(fieldsOf(form)).toBe("model=&input=&output=&cached=")
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }))

    fireEvent.click(screen.getByRole("button", { name: "Add address" }))
    dialog = await screen.findByRole("dialog", { name: "Add an address" })
    form = dialog.querySelector("form")!
    expect(form.getAttribute("action")).toBe(
      "/admin/models/providers/acme/addresses"
    )
    fireEvent.click(within(dialog).getByRole("button", { name: /^Custom:/ }))
    fireEvent.click(within(dialog).getByRole("checkbox", { name: "Messages" }))
    expect(fieldsOf(form)).toBe(
      "address=&kind=openai&auth=bearer&wires=messages"
    )
  })

  it("replaces a key kept here, and only one kept here", async () => {
    drawModels(acmeData(), acmePage)
    fireEvent.click(await screen.findByRole("button", { name: "Replace key" }))
    const dialog = await screen.findByRole("dialog")
    expect(dialog.querySelector("form")!.getAttribute("action")).toBe(
      "/admin/models/providers/acme/key"
    )
    expect(within(dialog).getByLabelText("New key").getAttribute("type")).toBe(
      "password"
    )
  })

  it("offers no new key for a key that is named, not kept", async () => {
    drawModels(
      acmeData((page) => (page.Provider!.Key = "file:/run/key")),
      acmePage
    )
    expect(await screen.findByText(/^Key: file:\/run\/key,/)).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Replace key" })).toBeNull()
  })
})

describe("one model of the organization's provider", () => {
  it("is priced from its menu, holding the price it has", async () => {
    drawModels(acmeData(), acmePage)
    const row = (await screen.findByText("acme/big-1")).closest("tr")!
    expect(row.textContent).toContain("$0.5")
    expect(row.textContent).toContain("$1.5")
    expect(row.textContent).toContain("as input")
    fireEvent.click(
      within(row).getByRole("button", { name: "More for acme/big-1" })
    )
    fireEvent.click(await screen.findByRole("menuitem", { name: "Set price" }))
    const dialog = await screen.findByRole("dialog")
    const form = dialog.querySelector("form")!
    expect(form.getAttribute("action")).toBe(
      "/admin/models/providers/acme/price"
    )
    expect(fieldsOf(form)).toBe("model=big-1&input=0.5&output=1.5&cached=")
  })

  it("is turned on and retired from its menu, each by a form of the row's", async () => {
    drawModels(acmeData(), acmePage)
    const row = (await screen.findByText("acme/big-1")).closest("tr")!
    const sent: string[] = []
    vi.spyOn(HTMLFormElement.prototype, "requestSubmit").mockImplementation(
      function (this: HTMLFormElement) {
        sent.push(`${this.getAttribute("action")} ${fieldsOf(this)}`)
      }
    )
    for (const item of ["Turn on", "Retire"]) {
      fireEvent.click(
        within(row).getByRole("button", { name: "More for acme/big-1" })
      )
      fireEvent.click(await screen.findByRole("menuitem", { name: item }))
    }
    expect(sent).toEqual([
      "/admin/models/providers/acme/models/enable model=big-1",
      "/admin/models/providers/acme/models/retire model=big-1",
    ])
  })

  it("says a provider has no models yet, and to add them", async () => {
    drawModels(
      acmeData((page) => (page.Models = null)),
      acmePage
    )
    expect(
      await screen.findByText("No models yet. Add the models acme answers.")
    ).toBeTruthy()
  })
})

describe("the operator's provider", () => {
  it("is shown with its prices and nothing to change", async () => {
    drawModels(
      acmeData((page) => {
        page.Provider = {
          ...acme,
          Name: "platform",
          Operator: true,
          Pending: false,
          Key: "the operator's",
          Addresses: [acme.Addresses![0]],
        }
        page.Models = [
          {
            Name: "qwen",
            Full: "platform/qwen",
            Price: {
              inputPerMillion: 1,
              outputPerMillion: 2,
              cachedInputPerMillion: 0,
            },
            Pending: false,
          },
          { Name: "free", Full: "platform/free", Price: null, Pending: false },
        ]
      }),
      "/admin/models?tab=providers&provider=platform"
    )
    expect(
      await screen.findByText("set by the operator of this gusto")
    ).toBeTruthy()
    const qwen = screen.getByText("platform/qwen").closest("tr")!
    expect(qwen.textContent).toBe("platform/qwen$1$2$0")
    expect(
      screen.getByText("no price, so its cost is not counted against a cap")
    ).toBeTruthy()
    for (const name of [
      /^More for/,
      /^Add (model|address)$/,
      /^Turn/,
      /^Retire/,
      /^Remove/,
    ]) {
      expect(screen.queryByRole("button", { name })).toBeNull()
    }
    expect(screen.queryByText(/the same at each address/)).toBeNull()
  })
})
