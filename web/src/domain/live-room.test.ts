import { describe, expect, it } from "vitest"

import {
  attach,
  meaningOf,
  ownerWords,
  pending,
  putTurn,
  typingWords,
} from "@/domain/live-room"
import type { LiveFrame, TurnView } from "@/pages/conversation/types"

const turn = (id: string, change: Partial<TurnView> = {}) =>
  ({
    ID: id,
    Kind: "turn",
    Done: true,
    Owner: "",
    OwnerName: "",
    ...change,
  }) as TurnView

const frame = (change: Partial<LiveFrame>): LiveFrame => ({
  v: 1,
  type: "turn",
  room: "r",
  thread: "1",
  seq: 1,
  id: "x",
  ver: "v",
  ...change,
})

describe("a live room", () => {
  it("puts a turn of this conversation in its place, and takes a snapshot when told", () => {
    const shown = turn("a")
    expect(meaningOf(frame({ turn: shown }), "1")).toEqual({
      kind: "put",
      turn: shown,
    })
    expect(meaningOf(frame({ turn: shown, thread: "2" }), "1")).toEqual({
      kind: "ignore",
    })
    expect(meaningOf(frame({ type: "people" }), "1")).toEqual({
      kind: "snapshot",
    })
    expect(meaningOf(frame({ type: "turn" }), "1")).toEqual({
      kind: "snapshot",
    })
    expect(meaningOf(frame({ v: 2 }), "1")).toEqual({ kind: "reload" })
    expect(meaningOf(frame({ type: "skip" }), "1")).toEqual({ kind: "ignore" })
  })

  it("replaces a turn it has, and adds a new one last", () => {
    const turns = [turn("a"), turn("b", { Done: false })]
    expect(putTurn(turns, turn("b")).map((each) => each.Done)).toEqual([
      true,
      true,
    ])
    expect(putTurn(turns, turn("c")).map((each) => each.ID)).toEqual([
      "a",
      "b",
      "c",
    ])
    expect(pending(turns)).toBe(true)
    expect(pending([turn("a"), turn("n", { Kind: "note", Done: false })])).toBe(
      false
    )
  })

  it("says whose agent replied as the person looking reads it", () => {
    expect(ownerWords(turn("a"), "me@x")).toBe("agent")
    expect(ownerWords(turn("a", { Owner: "Me@X" }), "me@x")).toBe("your agent")
    expect(
      ownerWords(turn("a", { Owner: "ada@x", OwnerName: "Ada" }), "me@x")
    ).toBe("Ada's agent")
  })

  it("names one or two people typing, and counts more", () => {
    expect(typingWords([])).toBe("")
    expect(typingWords(["Ada"])).toBe("Ada is typing…")
    expect(typingWords(["Ada", "Sam"])).toBe("Ada and Sam are typing…")
    expect(typingWords(["Ada", "Sam", "Bo"])).toBe("Several people are typing…")
  })

  it("refuses a file too big, one with an @ in its name, and one past the most", () => {
    const file = (name: string, size = 10) =>
      new File([new Uint8Array(size)], name)
    expect(attach([], [file("big.pdf", 20)], 5, 10).refused).toBe(
      "big.pdf is more than 10 bytes, so it was not attached."
    )
    expect(attach([], [file("a@b.txt")], 5, 10).refused).toMatch(
      /has an @ in its name/
    )
    const two = attach([file("1")], [file("2"), file("3")], 2, 100)
    expect(two.kept.map((each) => each.name)).toEqual(["1", "2"])
    expect(two.refused).toBe("A message takes at most 2 files.")
  })
})
