// A room as it changes while it is open: what each frame of its live channel means, how a turn takes its place among the
// others, whose agent replied, who is typing, and what may be attached to a message. Nothing here draws or connects.

import type { LiveFrame, TurnView } from "@/pages/conversation/types"

export type Meaning =
  | { kind: "put"; turn: TurnView }
  | { kind: "snapshot" }
  | { kind: "reload" }
  | { kind: "ignore" }

// meaningOf is what to do with a frame: put a turn in its place, take a fresh snapshot, reload a page that is out of date, or
// leave a frame for another conversation of the room alone.
export function meaningOf(frame: LiveFrame, thread: string): Meaning {
  if (frame.v !== 1) return { kind: "reload" }
  if (frame.type === "people" || frame.type === "refresh")
    return { kind: "snapshot" }
  if (frame.type !== "turn" && frame.type !== "note") return { kind: "ignore" }
  if (frame.thread && frame.thread !== thread) return { kind: "ignore" }
  return frame.turn ? { kind: "put", turn: frame.turn } : { kind: "snapshot" }
}

// putTurn is the turns with one put in its place: a turn already there is replaced, a new one comes last.
export function putTurn(turns: TurnView[], turn: TurnView): TurnView[] {
  const at = turns.findIndex((each) => each.ID === turn.ID)
  if (at < 0) return [...turns, turn]
  return turns.map((each, index) => (index === at ? turn : each))
}

// pending is whether a reply is still being written, which is when the page checks for what it may have missed more often.
export function pending(turns: TurnView[]): boolean {
  return turns.some((turn) => turn.Kind === "turn" && !turn.Done)
}

// ownerWords is whose agent replied, as the person looking reads it.
export function ownerWords(turn: TurnView, me: string): string {
  if (!turn.Owner) return "agent"
  return turn.Owner.toLowerCase() === me.toLowerCase()
    ? "your agent"
    : `${turn.OwnerName}'s agent`
}

// typingWords says who is typing, by name for one or two.
export function typingWords(names: string[]): string {
  if (names.length === 0) return ""
  if (names.length === 1) return `${names[0]} is typing…`
  if (names.length === 2) return `${names[0]} and ${names[1]} are typing…`
  return "Several people are typing…"
}

export function sizeWords(bytes: number): string {
  if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(1)} MB`
  if (bytes >= 1 << 10) return `${Math.round(bytes / (1 << 10))} KB`
  return bytes === 1 ? "1 byte" : `${bytes} bytes`
}

export interface Attaching {
  kept: File[]
  // refused is why the last file refused was, or nothing.
  refused: string
}

// attach adds files to those already attached, refusing one too big, one whose name holds an @ (which the room would read as a
// mention), and any past the most a message takes.
export function attach(
  attached: File[],
  adding: File[],
  most: number,
  biggest: number
): Attaching {
  const kept = [...attached]
  let refused = ""
  for (const file of adding) {
    if (file.size > biggest) {
      refused = `${file.name} is more than ${sizeWords(biggest)}, so it was not attached.`
    } else if (file.name.includes("@")) {
      refused = `${file.name} has an @ in its name, which the room would read as a mention. Rename it to attach it.`
    } else if (kept.length >= most) {
      refused = `A message takes at most ${most} files.`
    } else {
      kept.push(file)
    }
  }
  return { kept, refused }
}
