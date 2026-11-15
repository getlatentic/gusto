// What the file viewer says of a file and how it numbers the lines of one that is code. Nothing here draws.

import { ago, plural } from "@/lib/format"

// factsOf is the line under a file's name: its size, how many lines it has, and when it changed.
export function factsOf(
  file: { Size: string; Lines: number; Modified: number },
  now = Date.now()
): string {
  const facts = [file.Size]
  if (file.Lines) facts.push(`${file.Lines} ${plural(file.Lines, "line")}`)
  if (file.Modified) facts.push(`changed ${ago(file.Modified, now)}`)
  return facts.join(" · ")
}

export function escapeText(text: string): string {
  return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
}

// numberedLines splits code drawn as HTML into its lines. A colored stretch can run over several lines, so each line closes what it
// has open and the next one opens it again; a last line end makes no line of its own.
export function numberedLines(html: string): string[] {
  const open: string[] = []
  return html
    .replace(/\n$/, "")
    .split("\n")
    .map((line) => {
      const before = open.join("")
      for (const [tag, closing] of line.matchAll(/<(\/?)span[^>]*>/g)) {
        if (closing) open.pop()
        else open.push(tag)
      }
      return before + line + "</span>".repeat(open.length)
    })
}

// withSource is the address of a file's data asked for as its source.
export function withSource(address: string): string {
  return `${address}${address.includes("?") ? "&" : "?"}source=1`
}
