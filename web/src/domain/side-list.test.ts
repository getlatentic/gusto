import { describe, expect, it } from "vitest"

import {
  holds,
  isOpen,
  roomsForYou,
  unreadRooms,
  type RailChat,
  type RailList,
} from "@/domain/side-list"

const chat = (
  name: string,
  where: string,
  count = 0,
  forYou = 0
): RailChat => ({
  Name: name,
  Where: where,
  When: 0,
  Room: true,
  Unread: { Count: count, ForYou: forYou },
  ID: name,
  Harness: "",
  Mark: "",
  MarkDark: "",
})

const rail = (change: Partial<RailList>): RailList => ({
  Direct: null,
  Rooms: null,
  Archived: null,
  Latest: null,
  Unread: 0,
  ForYou: 0,
  Recent: null,
  Older: null,
  ...change,
})

describe("the side list", () => {
  it("counts a person's own rooms with an agent among the unread, as the server's counts do", () => {
    const list = rail({
      Direct: [chat("Scout", "/a", 2, 1)],
      Rooms: [chat("Launch", "/b", 1), chat("Quiet", "/c")],
    })
    expect(unreadRooms(list).map((room) => room.Name)).toEqual([
      "Scout",
      "Launch",
    ])
    expect(roomsForYou(list).map((room) => room.Name)).toEqual(["Scout"])
  })

  it("opens the first project, and the one holding the conversation open", () => {
    const first = { ID: "1", Name: "a", Detail: "", Runs: "", Chats: [] }
    const other = {
      ID: "2",
      Name: "b",
      Detail: "",
      Runs: "",
      Chats: [chat("x", "/here")],
    }
    expect(isOpen(first, 0, "/elsewhere")).toBe(true)
    expect(isOpen(other, 1, "/elsewhere")).toBe(false)
    expect(isOpen(other, 1, "/here")).toBe(true)
    expect(holds([], "/here")).toBe(false)
  })
})
