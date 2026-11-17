import { useGSAP } from "@gsap/react"
import { gsap } from "gsap"
import { Flip } from "gsap/Flip"

gsap.registerPlugin(useGSAP, Flip)

export { Flip, gsap, useGSAP }
export { moves } from "@/motion/calm"

// The app's motion is short, so it reads as the interface answering rather than as decoration.
export const quick = 0.16
export const settle = 0.24
export const ease = "power2.out"
