import { renderPage } from "@/test/render"
import { ModelsPage } from "./models-page"
import type { ModelsData, Provider } from "./types"

// The Models page's data for one tab, as a server with the operator's qwen and the organization's acme gives it.
export function modelsData(
  tab: string,
  change: (page: ModelsData) => void = () => {}
): ModelsData {
  const keys = ["models", "providers", "access", "settings"]
  const labels = ["Models", "Providers", "Who may use", "Settings"]
  const page: ModelsData = {
    View: {
      Kinds: ["stored", "file"],
      Available: ["platform/qwen", "acme/big-1"],
      Private: false,
      Editable: false,
      Unnamed: false,
    },
    Refused: "",
    Tab: tab,
    Tabs: keys.map((key, i) => ({
      Key: key,
      Label: labels[i],
      Href: `/admin/models?tab=${key}`,
      On: key === tab,
    })),
    Pages: { Shown: "", Previous: "", Next: "" },
  }
  change(page)
  return page
}

export const acme: Provider = {
  Name: "acme",
  Addresses: [
    {
      Address: "https://api.theirs.example",
      Kind: "OpenAI-compatible",
      Extra: false,
    },
    {
      Address: "https://api.theirs.example/anthropic",
      Kind: "Anthropic-compatible",
      Extra: true,
    },
  ],
  Operator: false,
  Pending: true,
  Key: "stored",
  Problem: "",
}

// drawModels draws the Models page at an address, with the server answering it the data given.
export function drawModels(page: ModelsData, at = "/admin/models") {
  return renderPage(at, "/admin/models", <ModelsPage />, { [at]: page })
}

// formOf is the form a control is in.
export function formOf(element: HTMLElement): HTMLFormElement {
  const form = element.closest("form")
  if (!form) throw new Error("the control is in no form")
  return form
}

// fieldsOf is what a form would send, as the browser sends it.
export function fieldsOf(form: HTMLFormElement): string {
  const fields = new URLSearchParams()
  for (const [key, value] of new FormData(form)) {
    if (typeof value === "string") fields.append(key, value)
  }
  return fields.toString()
}
