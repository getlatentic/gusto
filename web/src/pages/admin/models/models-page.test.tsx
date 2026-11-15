import { fireEvent, screen, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { drawModels, fieldsOf, modelsData } from "./test-data"

const rows = [
  {
    Name: "Qwen",
    Full: "platform/qwen",
    Provider: "platform",
    Input: "$1.00",
    Output: "$2.00",
    Who: "everyone",
    Off: false,
  },
  {
    Name: "big-1",
    Full: "acme/big-1",
    Provider: "acme",
    Input: "",
    Output: "",
    Who: "everyone but Bob",
    Off: true,
  },
]

describe("the Models tab", () => {
  it("lists each model with its price and who may use it, under the page's tabs", async () => {
    drawModels(modelsData("models", (page) => (page.Rows = rows)))
    const qwen = (await screen.findByTitle("platform/qwen")).closest("tr")!
    expect(qwen.textContent).toContain("Qwen")
    expect(qwen.textContent).toContain("$1.00")
    expect(qwen.textContent).toContain("$2.00")
    const big = screen.getByTitle("acme/big-1").closest("tr")!
    expect(big.textContent).toContain("off")
    expect(big.textContent).toContain("no price")
    expect(big.textContent).toContain("everyone but Bob")
    expect(
      screen.getByText(
        "Where models answer, what they cost, and who may use them"
      )
    ).toBeTruthy()
    const current = screen.getByRole("tab", { name: "Models" })
    expect(current.getAttribute("aria-selected")).toBe("true")
    expect(
      screen.getByRole("tab", { name: "Who may use" }).getAttribute("href")
    ).toBe("/admin/models?tab=access")
    expect(screen.queryByText(/Registry version/)).toBeNull()
  })

  it("leads to the next page of a long table", async () => {
    drawModels(
      modelsData("models", (page) => {
        page.Rows = rows
        page.Pages = {
          Shown: "1–25 of 31",
          Previous: "",
          Next: "/admin/models?tab=models&page=2",
        }
      })
    )
    const next = await screen.findByRole("link", { name: "Next" })
    expect(next.getAttribute("href")).toBe("/admin/models?tab=models&page=2")
    expect(next.getAttribute("rel")).toBe("next")
    expect(screen.getByText("1–25 of 31")).toBeTruthy()
  })

  it("says there are no models yet, and how to have one", async () => {
    drawModels(modelsData("models"))
    expect(
      await screen.findByText(
        "No models yet. Add a provider, then the models it answers."
      )
    ).toBeTruthy()
  })

  it("says why a change was refused", async () => {
    const at = "/admin/models?tab=models&refused=x"
    drawModels(
      modelsData(
        "models",
        (page) => (page.Refused = "a provider is reached over https")
      ),
      at
    )
    expect((await screen.findByRole("alert")).textContent).toBe(
      "a provider is reached over https"
    )
  })
})

describe("adding a provider", () => {
  it("is a dialog from the bar with where it answers and where its key comes from", async () => {
    drawModels(modelsData("models"))
    fireEvent.click(
      await screen.findByRole("button", { name: "Add a provider" })
    )
    const dialog = await screen.findByRole("dialog")
    const form = dialog.querySelector("form")!
    expect(form.getAttribute("action")).toBe("/admin/models/providers")
    expect(form.getAttribute("method")).toBe("post")
    expect(
      within(dialog).getByText("It must be at a public https address.")
    ).toBeTruthy()
    fireEvent.change(within(dialog).getByLabelText("Name"), {
      target: { value: "acme" },
    })
    fireEvent.change(within(dialog).getByLabelText("Address"), {
      target: { value: "https://api.theirs.example" },
    })
    fireEvent.change(within(dialog).getByLabelText("Key"), {
      target: { value: "sk-1" },
    })
    expect(fieldsOf(form)).toBe(
      "name=acme&address=https%3A%2F%2Fapi.theirs.example&kind=openai&auth=bearer&source=stored&key=sk-1&reference="
    )
    expect(within(dialog).getByLabelText("Key").getAttribute("type")).toBe(
      "password"
    )
    expect(
      within(dialog).getByText("typed here, kept and never shown")
    ).toBeTruthy()
  })

  it("is not offered beside the operator's provider with no name", async () => {
    drawModels(modelsData("providers", (page) => (page.View.Unnamed = true)))
    expect(await screen.findByText(/cannot be added beside it/)).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Add a provider" })).toBeNull()
  })

  it("does not say an address must be public where private ones are allowed", async () => {
    drawModels(modelsData("models", (page) => (page.View.Private = true)))
    fireEvent.click(
      await screen.findByRole("button", { name: "Add a provider" })
    )
    await screen.findByRole("dialog")
    expect(
      screen.queryByText("It must be at a public https address.")
    ).toBeNull()
  })
})
