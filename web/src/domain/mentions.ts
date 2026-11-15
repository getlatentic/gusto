// Mentions in a room's message box: an @ just before the caret asks for someone, the room's agents and people whose names start
// with what was typed are offered, and choosing one writes "@Name " in its place. A mention is an @ and someone's whole name,
// which can hold spaces; where two names begin alike, the longest wins. Nothing here draws.

import type { Mentionable } from "@/pages/conversation/types"

// asking is an @ that starts a word, then up to 32 characters with no @ or new line, at the end of what is before the caret.
const asking = /(^|[^\w@])@([^@\n]{0,32})$/

export interface Asking {
  // at is where the @ is.
  at: number
  typed: string
}

// askingAt is what is being asked for at the caret, or nothing when the caret is not after an @.
export function askingAt(text: string, caret: number): Asking | undefined {
  const before = text.slice(0, caret)
  const found = asking.exec(before)
  if (!found) return undefined
  return { at: before.length - found[2].length - 1, typed: found[2] }
}

// offered are those whose name starts with what was typed, agents first, and not the one whose whole name it already is.
export function offered(everyone: Mentionable[], typed: string): Mentionable[] {
  const asked = typed.toLowerCase()
  const fits = everyone.filter((each) => {
    const name = each.name.toLowerCase()
    return name.startsWith(asked) && name !== asked.trimEnd()
  })
  return [
    ...fits.filter((each) => each.agent),
    ...fits.filter((each) => !each.agent),
  ]
}

// mention writes "@Name " over what was typed, and says where the caret goes after it.
export function mention(
  text: string,
  asked: Asking,
  caret: number,
  name: string
): { text: string; caret: number } {
  const put = `@${name} `
  return {
    text: text.slice(0, asked.at) + put + text.slice(caret),
    caret: asked.at + put.length,
  }
}

export interface Piece {
  text: string
  mention: boolean
}

const escaped = (name: string) => name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")

// pieces is a message cut into its plain text and the mentions in it, for drawing the mentions in colour as they are typed.
export function pieces(text: string, names: string[]): Piece[] {
  if (names.length === 0 || text === "") return [{ text, mention: false }]
  const longestFirst = [...names]
    .sort((a, b) => b.length - a.length)
    .map(escaped)
  const found = new RegExp(
    `(^|[^\\p{L}\\p{N}_@])(@(?:${longestFirst.join("|")}))(?![\\p{L}\\p{N}_])`,
    "giu"
  )
  const out: Piece[] = []
  let from = 0
  for (const match of text.matchAll(found)) {
    const start = match.index + match[1].length
    if (start > from)
      out.push({ text: text.slice(from, start), mention: false })
    out.push({ text: match[2], mention: true })
    from = start + match[2].length
  }
  if (from < text.length) out.push({ text: text.slice(from), mention: false })
  return out
}
