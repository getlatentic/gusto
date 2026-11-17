import { useLayoutEffect, useRef, type RefObject } from "react"

import { cn } from "@/lib/utils"

// labelRoom is how near the right edge, in pixels, the pointer's label moves to its left.
const labelRoom = 220

// DemoCursor is the pointer that shows what the demo clicks. It glides to the middle of the stage's element marked
// data-demo-target with the target's name, shows label beside it, and rings when it clicks. Positions are in the stage's own
// pixels, before it is scaled to fit.
export function DemoCursor({
  stage,
  scale,
  target,
  label,
  clicking,
}: {
  stage: RefObject<HTMLElement | null>
  scale: number
  target: string | null
  label: string
  clicking: boolean
}) {
  const pointer = useRef<HTMLDivElement>(null)
  const note = useRef<HTMLSpanElement>(null)
  // Where the target is depends on the page drawn, which changes as the scene plays, so it is measured after each draw and
  // written straight to the pointer's style, where the transition glides it there.
  useLayoutEffect(() => {
    const root = stage.current
    const at = pointer.current
    const element = target
      ? root?.querySelector<HTMLElement>(`[data-demo-target="${target}"]`)
      : null
    if (!root || !at || !element) return
    const from = root.getBoundingClientRect()
    const box = element.getBoundingClientRect()
    // A phone page has no side panel, so a step that clicks in it has nothing to point at.
    if (box.width === 0 && box.height === 0) {
      at.style.opacity = "0"
      return
    }
    const x = Math.round((box.left - from.left + box.width / 2) / scale)
    at.style.left = `${x}px`
    at.style.top = `${Math.round((box.top - from.top + box.height / 2) / scale)}px`
    at.style.opacity = "1"
    // Near the right edge the label sits to the pointer's left, so the window never cuts it off.
    if (note.current) {
      const flip = x > root.clientWidth - labelRoom
      note.current.style.left = flip ? "auto" : "16px"
      note.current.style.right = flip ? "28px" : "auto"
    }
  })
  return (
    <div
      ref={pointer}
      className="pointer-events-none absolute z-10 opacity-0 transition-[left,top] duration-700 ease-in-out"
      data-testid="demo-cursor"
    >
      <span
        className={cn(
          "absolute -top-4 -left-4 size-8 rounded-full border-2 border-solid transition-[opacity,scale] duration-300",
          clicking ? "scale-100 opacity-60" : "scale-50 opacity-0"
        )}
      />
      <svg
        viewBox="0 0 24 24"
        className="fill-ink stroke-canvas relative -mt-0.5 -ml-1 size-6 drop-shadow"
        strokeWidth={1.5}
      >
        <path d="M4 3l15 8.5-6.5 1.5L9.5 19z" strokeLinejoin="round" />
      </svg>
      {label && (
        <span
          ref={note}
          className="bg-ink text-canvas absolute top-6 left-4 rounded-full px-2.5 py-1 text-xs font-medium whitespace-nowrap shadow-md"
        >
          {label}
        </span>
      )}
    </div>
  )
}
