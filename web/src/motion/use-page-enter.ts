import { useRef, type RefObject } from "react"

import { ease, gsap, moves, quick, useGSAP } from "@/motion/gsap"

// usePageEnter fades the page in a container in when it changes to another, which key names. The first page drawn appears as
// it is, as does a page that only changed its query, such as a filter.
export function usePageEnter(
  container: RefObject<HTMLElement | null>,
  key: string
) {
  const shown = useRef<string | null>(null)
  useGSAP(
    () => {
      const before = shown.current
      shown.current = key
      if (before === null || before === key || !container.current) return
      if (!moves()) return
      gsap.fromTo(
        container.current,
        { opacity: 0, y: 6 },
        {
          opacity: 1,
          y: 0,
          duration: quick,
          ease,
          overwrite: true,
          clearProps: "opacity,transform",
        }
      )
    },
    { dependencies: [key] }
  )
}
