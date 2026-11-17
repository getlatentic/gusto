// A scene of the landing page's demo is a script: a starting state and timed steps that move a cursor, click, type, and change
// what the scene shows. What is on screen at any moment is worked out from the time alone, so a scene can start again, jump to
// its end for a person who asked for less motion, or be tested, without any animation running.

export interface Step<S extends object> {
  // at is when the step happens, in milliseconds from the start of the scene.
  at: number
  // cursor moves the pointer to the middle of the scene's element marked data-demo-target with this name, with label beside it
  // naming what it is about to do.
  cursor?: string
  label?: string
  click?: boolean
  set?: Partial<S>
  // type writes text into one of the state's fields a character at a time.
  type?: { key: keyof S; text: string; perChar: number }
}

export interface Scene<S extends object> {
  // length is how long the scene plays before the next one starts.
  length: number
  start: S
  steps: Step<S>[]
}

export interface Frame<S extends object> {
  state: S
  cursor: string | null
  label: string
  clicking: boolean
}

// clickLasts is how long the ring of a click shows.
const clickLasts = 320

// frameAt is what a scene shows at ms milliseconds in. Steps not yet reached change nothing; a step being typed shows the
// characters typed so far.
export function frameAt<S extends object>(
  scene: Scene<S>,
  ms: number
): Frame<S> {
  const state = { ...scene.start }
  let cursor: string | null = null
  let label = ""
  let clicking = false
  for (const step of scene.steps) {
    if (step.at > ms) break
    if (step.cursor) {
      cursor = step.cursor
      label = step.label ?? ""
    }
    if (step.click) clicking = ms - step.at < clickLasts
    if (step.set) Object.assign(state, step.set)
    if (step.type) {
      const typed = Math.floor((ms - step.at) / step.type.perChar)
      Object.assign(state, {
        [step.type.key]: step.type.text.slice(0, Math.max(0, typed)),
      })
    }
  }
  return { state, cursor, label, clicking }
}
