// A panel's width, in rem: a person drags it between its bounds, and it never takes more than its share of the window.

export interface Bounds {
  least: number
  most: number
  usual: number
  // share is the most of the window's width the panel may take.
  share: number
}

// sidePanel is the list of conversations; filePanel is the files open beside a conversation.
export const sidePanel: Bounds = { least: 16, most: 28, usual: 20, share: 0.4 }
export const filePanel: Bounds = { least: 22, most: 60, usual: 36, share: 0.7 }

export function clampWidth(
  rem: number,
  windowRem: number,
  bounds: Bounds
): number {
  const most = Math.min(bounds.most, windowRem * bounds.share)
  return Math.round(Math.max(bounds.least, Math.min(most, rem)) * 4) / 4
}

// keptWidth is a width kept as text, or the usual one when nothing usable was kept.
export function keptWidth(kept: string | null, bounds: Bounds): number {
  const rem = Number.parseFloat(kept ?? "")
  return Number.isFinite(rem) ? rem : bounds.usual
}
