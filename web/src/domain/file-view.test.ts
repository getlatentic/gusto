import { describe, expect, it } from "vitest"

import {
  escapeText,
  factsOf,
  numberedLines,
  withSource,
} from "@/domain/file-view"

describe("the file viewer", () => {
  it("says a file's size, its lines and when it changed", () => {
    const now = 1_000_000_000_000
    expect(
      factsOf({ Size: "12 bytes", Lines: 1, Modified: now / 1000 - 120 }, now)
    ).toBe("12 bytes · 1 line · changed 2m ago")
    expect(factsOf({ Size: "3.0 KiB", Lines: 4, Modified: 0 }, now)).toBe(
      "3.0 KiB · 4 lines"
    )
    expect(factsOf({ Size: "1 bytes", Lines: 0, Modified: 0 }, now)).toBe(
      "1 bytes"
    )
  })

  it("numbers code by its lines, carrying a colored stretch over the line ends", () => {
    expect(
      numberedLines('a\n<span class="s">"one\ntwo"</span> b\nc\n')
    ).toEqual([
      "a",
      '<span class="s">"one</span>',
      '<span class="s">two"</span> b',
      "c",
    ])
    expect(
      numberedLines('<span class="x"><span class="y">a\nb</span></span>')
    ).toEqual([
      '<span class="x"><span class="y">a</span></span>',
      '<span class="x"><span class="y">b</span></span>',
    ])
  })

  it("escapes text drawn as HTML, and asks for a page's source", () => {
    expect(escapeText("<b>&</b>")).toBe("&lt;b&gt;&amp;&lt;/b&gt;")
    expect(withSource("/app/api/instances/a/file?path=x.html")).toBe(
      "/app/api/instances/a/file?path=x.html&source=1"
    )
  })
})
