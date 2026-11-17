import { useRef, type RefObject } from "react"

import { ease, gsap, moves, settle, useGSAP } from "@/motion/gsap"

// useArrivals brings in each element of a container marked data-arrive-id whose id was not there before, such as a message or a
// step of a reply as it comes. What was there when the container was first drawn appears as it is.
export function useArrivals(
  container: RefObject<HTMLElement | null>,
  ids: readonly string[]
) {
  const seen = useRef<Set<string> | null>(null)
  useGSAP(
    () => {
      const before = seen.current
      seen.current = new Set(ids)
      if (!before || !container.current || !moves()) return
      const fresh = [
        ...container.current.querySelectorAll<HTMLElement>("[data-arrive-id]"),
      ].filter((element) => !before.has(element.dataset.arriveId ?? ""))
      if (fresh.length === 0) return
      gsap.from(fresh, {
        opacity: 0,
        y: 8,
        duration: settle,
        ease,
        stagger: 0.04,
        clearProps: "opacity,transform",
      })
    },
    { dependencies: [ids.join("\n")] }
  )
}
