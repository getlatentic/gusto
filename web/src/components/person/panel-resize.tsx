import { useState, type KeyboardEvent, type PointerEvent } from "react"

import { clampWidth, keptWidth, panelWidth } from "@/domain/panel-width"

const widthKey = "gusto-panel-w"

function readKept(): number {
  try {
    return keptWidth(localStorage.getItem(widthKey))
  } catch {
    return panelWidth.usual
  }
}

function keep(rem: number) {
  try {
    localStorage.setItem(widthKey, String(rem))
  } catch {
    // A browser that keeps nothing starts at the usual width next time.
  }
}

const remPx = () =>
  Number.parseFloat(getComputedStyle(document.documentElement).fontSize) || 16

// usePanelWidth is the side panel's width in rem, as this device keeps it, and the handlers of the divider that changes it:
// dragged, moved a rem at a time by the arrow keys, or put back by a double click.
export function usePanelWidth() {
  const [width, setWidth] = useState(readKept)
  const within = (rem: number) => clampWidth(rem, window.innerWidth / remPx())
  const settle = (rem: number) => {
    const next = within(rem)
    setWidth(next)
    keep(next)
  }
  const divider = {
    onPointerDown(event: PointerEvent<HTMLElement>) {
      event.currentTarget.setPointerCapture(event.pointerId)
    },
    onPointerMove(event: PointerEvent<HTMLElement>) {
      if (!event.currentTarget.hasPointerCapture(event.pointerId)) return
      const left =
        event.currentTarget.parentElement?.getBoundingClientRect().left ?? 0
      // The panel is beside the rail, which is 3.25rem wide.
      setWidth(within((event.clientX - left) / remPx() - 3.25))
    },
    onPointerUp(event: PointerEvent<HTMLElement>) {
      event.currentTarget.releasePointerCapture(event.pointerId)
      keep(width)
    },
    onKeyDown(event: KeyboardEvent<HTMLElement>) {
      const by = { ArrowLeft: -1, ArrowRight: 1 }[event.key]
      if (by === undefined) return
      event.preventDefault()
      settle(width + by)
    },
    onDoubleClick() {
      settle(panelWidth.usual)
    },
  }
  return { width, divider }
}
