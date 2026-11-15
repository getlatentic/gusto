// The theme, accent and text size are the person's, kept on this device by look.js in the page head, which sets them before
// anything is drawn. This is its typed face; without it (as under test) the look stays as drawn.

export type Theme = "system" | "light" | "dark"

export interface Look {
  theme: Theme
  accent: string
  zoom: number
}

interface GustoLook {
  theme(): Theme
  accent(): string
  zoom(): number
  stepZoom(by: number): void
  setTheme(value: Theme): void
  setAccent(value: string): void
}

const look = (): GustoLook | undefined =>
  (window as unknown as { gustoLook?: GustoLook }).gustoLook

export function readLook(): Look {
  const kept = look()
  return kept
    ? { theme: kept.theme(), accent: kept.accent(), zoom: kept.zoom() }
    : { theme: "system", accent: "green", zoom: 100 }
}

export function setTheme(value: Theme): Look {
  look()?.setTheme(value)
  return readLook()
}

export function setAccent(value: string): Look {
  look()?.setAccent(value)
  return readLook()
}

export function stepZoom(by: number): Look {
  look()?.stepZoom(by)
  return readLook()
}

// accents are the colours a person may pick, as the tokens name them.
export const accents = [
  { key: "graphite", label: "Graphite", swatch: "#2B2B29" },
  { key: "orange", label: "Orange", swatch: "#FF8A1F" },
  { key: "blue", label: "Blue", swatch: "#3B6FE0" },
  { key: "green", label: "Green", swatch: "#23805A" },
  { key: "violet", label: "Violet", swatch: "#7352D4" },
  { key: "rose", label: "Rose", swatch: "#D23F67" },
]
