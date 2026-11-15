import type { ServedPage } from "@/cloud/types"

// readPage is the page the server wrote into this one, as JSON in the script it names gusto-data.
export function readPage(from: Document = document): ServedPage {
  const text = from.getElementById("gusto-data")?.textContent
  if (!text) throw new Error("this page was served without its data")
  return JSON.parse(text) as ServedPage
}
