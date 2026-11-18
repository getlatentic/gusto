import { useEffect, useReducer } from "react"

// tick is how often the demo's time moves on, in milliseconds: often enough for typing to look smooth.
const tick = 50

export interface Clock {
  scene: number
  ms: number
  paused: boolean
  // chosen is whether the person has picked a scene or paused, after which the demo no longer moves on by itself.
  chosen: boolean
}

export type ClockEvent =
  | { kind: "tick"; lengths: number[] }
  | { kind: "choose"; scene: number }
  | { kind: "pause" }

export const startClock: Clock = {
  scene: 0,
  ms: 0,
  paused: false,
  chosen: false,
}

// clockAfter is where the demo is after an event. Left alone, a tick moves time on and, at the end of a scene, starts the
// next, after the last the first. Once the person has chosen, a scene that ends holds on its last moment.
export function clockAfter(clock: Clock, event: ClockEvent): Clock {
  switch (event.kind) {
    case "choose":
      return { ...clock, scene: event.scene, ms: 0, chosen: true }
    case "pause":
      return { ...clock, paused: !clock.paused, chosen: true }
    case "tick": {
      const length = event.lengths[clock.scene]
      if (clock.ms + tick < length) return { ...clock, ms: clock.ms + tick }
      if (clock.chosen) return { ...clock, ms: length }
      return {
        ...clock,
        scene: (clock.scene + 1) % event.lengths.length,
        ms: 0,
      }
    }
  }
}

// useDemoClock runs the clock while running is true, which the caller sets from whether the demo is on screen.
export function useDemoClock(lengths: number[], running: boolean) {
  const [clock, send] = useReducer(clockAfter, startClock)
  const moving = running && !clock.paused
  useEffect(() => {
    if (!moving) return
    const timer = window.setInterval(
      () => send({ kind: "tick", lengths }),
      tick
    )
    return () => window.clearInterval(timer)
  }, [moving, lengths])
  return {
    clock,
    choose: (scene: number) => send({ kind: "choose", scene }),
    togglePause: () => send({ kind: "pause" }),
  }
}
