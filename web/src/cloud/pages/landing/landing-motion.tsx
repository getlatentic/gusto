import { useGSAP } from "@gsap/react"
import { gsap } from "gsap"
import { ScrollTrigger } from "gsap/ScrollTrigger"

import { calm } from "@/motion/calm"

gsap.registerPlugin(useGSAP, ScrollTrigger)

// LandingMotion moves the landing page as it is scrolled: each band's title and cards rise in as they come into view, once. It
// is fetched after the page is drawn, so it hides only what is still
// below the fold and never makes something already on screen blink. A person who asked for less motion sees the page as it
// is, and if they ask while it is open, gsap.matchMedia puts back everything it set. The landing page is the whole document, so
// it looks for its parts there.
export function LandingMotion() {
  useGSAP(() => {
    gsap.matchMedia().add(calm, () => {
      const below = gsap.utils
        .toArray<HTMLElement>("[data-reveal]")
        .filter((each) => each.getBoundingClientRect().top > innerHeight)
      if (below.length > 0) reveal(below)
    })
  })
  return null
}

// reveal hides the elements given and brings each batch of them in as it scrolls into view, once.
function reveal(elements: HTMLElement[]) {
  gsap.set(elements, { opacity: 0, y: 24 })
  ScrollTrigger.batch(elements, {
    start: "top 90%",
    once: true,
    onEnter: (shown) =>
      gsap.to(shown, {
        opacity: 1,
        y: 0,
        duration: 0.5,
        ease: "power3.out",
        stagger: 0.08,
        overwrite: true,
      }),
  })
}
