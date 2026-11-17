import { useEffect, useState, type RefObject } from "react"

import {
  compactHeight,
  compactWidth,
  demoHeight,
  demoWidth,
} from "@/cloud/pages/landing/demo/demo-parts"

// useOnScreen is whether enough of the element is in view, in a tab the person is looking at, for the demo to be worth playing.
export function useOnScreen(element: RefObject<HTMLElement | null>): boolean {
  const [inView, setInView] = useState(false)
  const [looking, setLooking] = useState(() => !document.hidden)
  useEffect(() => {
    const at = element.current
    if (!at) return
    const watch = new IntersectionObserver(
      ([seen]) => setInView(seen.isIntersecting),
      { threshold: 0.3 }
    )
    watch.observe(at)
    return () => watch.disconnect()
  }, [element])
  useEffect(() => {
    const follow = () => setLooking(!document.hidden)
    document.addEventListener("visibilitychange", follow)
    return () => document.removeEventListener("visibilitychange", follow)
  }, [])
  return inView && looking
}

// pageGutters is the landing page's margin either side of its column, together.
const pageGutters = 40

export interface DemoSize {
  // compact is whether the demo is drawn as a phone page, at the element's own width.
  compact: boolean
  width: number
  height: number
  // scale is how much a demo drawn at full size is shrunk to fit; a compact one is not.
  scale: number
}

// useDemoSize is how the demo fits the element: at full size, shrunk to the element's width, or, on a page narrower than
// compactWidth, drawn as a phone page at the element's own width. It never grows past its full size.
export function useDemoSize(element: RefObject<HTMLElement | null>): DemoSize {
  // The first guess is the window's width less the page's margins, so the demo is drawn to fit before the element has been
  // measured, which in a browser that holds back resize notices may be a while.
  const [width, setWidth] = useState(
    () => Math.min(window.innerWidth, demoWidth + pageGutters) - pageGutters
  )
  useEffect(() => {
    const at = element.current
    if (!at) return
    const fit = () => setWidth(at.clientWidth)
    fit()
    const watch = new ResizeObserver(fit)
    watch.observe(at)
    return () => watch.disconnect()
  }, [element])
  if (width > 0 && width < compactWidth) {
    return { compact: true, width, height: compactHeight, scale: 1 }
  }
  return {
    compact: false,
    width: demoWidth,
    height: demoHeight,
    scale: width > 0 ? Math.min(1, width / demoWidth) : 1,
  }
}
