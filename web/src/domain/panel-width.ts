// The side panel's width, in rem: a person drags it between the two bounds, and it never takes more than 40% of the window.

export const panelWidth = { least: 16, most: 28, usual: 20 }

export function clampWidth(rem: number, windowRem: number): number {
  const most = Math.min(panelWidth.most, windowRem * 0.4)
  return Math.round(Math.max(panelWidth.least, Math.min(most, rem)) * 4) / 4
}

// keptWidth is a width kept as text, or the usual one when nothing usable was kept.
export function keptWidth(kept: string | null): number {
  const rem = Number.parseFloat(kept ?? "")
  return Number.isFinite(rem) ? rem : panelWidth.usual
}
