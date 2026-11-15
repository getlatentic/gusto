// How an agent's mark is drawn: as its owner gives it, with no box. A mark that is black has a white version for a dark page,
// or is inverted there.

export interface MarkLook {
  light: string
  dark?: string
  invert: boolean
  // blend lets a mark with a white ground sit on the page as if it had none.
  blend: boolean
}

const hasDarkFile = /\/agents\/(opencode|pi)\.svg(\?.*)?$/
const inverts = /\/agents\/(codex|hermes)\./
const blends = /\/agents\/hermes\./

export function markLook(src: string): MarkLook {
  if (hasDarkFile.test(src)) {
    return {
      light: src,
      dark: src.replace(/\.svg(\?.*)?$/, "-dark.svg"),
      invert: false,
      blend: false,
    }
  }
  return { light: src, invert: inverts.test(src), blend: blends.test(src) }
}
