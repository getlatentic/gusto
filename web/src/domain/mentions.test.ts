import { describe, expect, it } from "vitest"

import { askingAt, mention, offered, pieces } from "@/domain/mentions"
import type { Mentionable } from "@/pages/conversation/types"

const one = (name: string, agent: boolean): Mentionable => ({
  name,
  role: "",
  owner: "",
  agent,
  icon: "",
  short: "",
})

const room = [
  one("Ada Lovelace", false),
  one("Scout", true),
  one("Scribe", true),
  one("Sam", false),
]

describe("mentioning someone in a room", () => {
  it("asks for someone only after an @ that starts a word", () => {
    expect(askingAt("hi @Sc", 6)).toEqual({ at: 3, typed: "Sc" })
    expect(askingAt("@", 1)).toEqual({ at: 0, typed: "" })
    expect(askingAt("mail me@ex", 10)).toBeUndefined()
    expect(askingAt("hi @Sc there", 6)).toEqual({ at: 3, typed: "Sc" })
  })

  it("offers agents first, by the start of their name, and not someone already named in full", () => {
    expect(offered(room, "s").map((each) => each.name)).toEqual([
      "Scout",
      "Scribe",
      "Sam",
    ])
    expect(offered(room, "ada l").map((each) => each.name)).toEqual([
      "Ada Lovelace",
    ])
    expect(offered(room, "Scout")).toEqual([])
  })

  it("writes the whole name with a space after, and puts the caret after it", () => {
    const text = "ask @Sc about it"
    expect(mention(text, { at: 4, typed: "Sc" }, 7, "Scout")).toEqual({
      text: "ask @Scout  about it",
      caret: 11,
    })
  })

  it("finds whole names, the longest first, and not an @ inside a word", () => {
    expect(
      pieces("hi @Ada Lovelace and @Scout, not a@Scout", [
        "Ada Lovelace",
        "Ada",
        "Scout",
      ])
    ).toEqual([
      { text: "hi ", mention: false },
      { text: "@Ada Lovelace", mention: true },
      { text: " and ", mention: false },
      { text: "@Scout", mention: true },
      { text: ", not a@Scout", mention: false },
    ])
    expect(pieces("@Scouting", ["Scout"])).toEqual([
      { text: "@Scouting", mention: false },
    ])
  })
})
