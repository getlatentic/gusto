// calm is the media query of a person who has not asked for less motion. GSAP sets styles inline, which the stylesheet's
// reduced-motion rule cannot reach, so every animation asks this first.
export const calm = "(prefers-reduced-motion: no-preference)"

export function moves(): boolean {
  return window.matchMedia(calm).matches
}
