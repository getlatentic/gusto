import { useState, type KeyboardEvent, type PointerEvent } from "react"

import { clampWidth, keptWidth, type Bounds } from "@/domain/panel-width"

function readKept(key: string, bounds: Bounds): number {
  try {
    return keptWidth(localStorage.getItem(key), bounds)
  } catch {
    return bounds.usual
  }
}

function keep(key: string, rem: number) {
  try {
    localStorage.setItem(key, String(rem))
  } catch {
    // A browser that keeps nothing starts at the usual width next time.
  }
}

const remPx = () =>
  Number.parseFloat(getComputedStyle(document.documentElement).fontSize) || 16

// usePanelWidth is a panel's width in rem, as this device keeps it under key, and the handlers of the divider that changes it:
// dragged, moved a rem at a time by the arrow keys, or put back by a double click. edge is the side of the panel its divider is
// on, which is the way it grows; measure says how wide the panel is, in pixels, with the pointer where it is.
export function usePanelWidth(
  key: string,
  bounds: Bounds,
  edge: "left" | "right",
  measure: (event: PointerEvent<HTMLElement>) => number
) {
  const [width, setWidth] = useState(() => readKept(key, bounds))
  const within = (rem: number) =>
    clampWidth(rem, window.innerWidth / remPx(), bounds)
  const settle = (rem: number) => {
    const next = within(rem)
    setWidth(next)
    keep(key, next)
  }
  const grows = edge === "right" ? 1 : -1
  const divider = {
    onPointerDown(event: PointerEvent<HTMLElement>) {
      event.currentTarget.setPointerCapture(event.pointerId)
    },
    onPointerMove(event: PointerEvent<HTMLElement>) {
      if (!event.currentTarget.hasPointerCapture(event.pointerId)) return
      setWidth(within(measure(event) / remPx()))
    },
    onPointerUp(event: PointerEvent<HTMLElement>) {
      event.currentTarget.releasePointerCapture(event.pointerId)
      keep(key, width)
    },
    onKeyDown(event: KeyboardEvent<HTMLElement>) {
      const by = { ArrowLeft: -grows, ArrowRight: grows }[event.key]
      if (by === undefined) return
      event.preventDefault()
      settle(width + by)
    },
    onDoubleClick() {
      settle(bounds.usual)
    },
  }
  return { width, divider }
}
