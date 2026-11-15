import { describe, expect, it } from "vitest"

import { rooms } from "@/store/rooms"

const { connecting, connected, removed, present, typing, closed } =
  rooms.actions

describe("a room's connection", () => {
  it("connects, goes live, drops, and stays removed once removed", () => {
    let state = rooms.reducer(undefined, connecting("r"))
    expect(state.r.connection).toBe("connecting")
    state = rooms.reducer(state, connected({ room: "r", live: true }))
    expect(state.r.connection).toBe("live")
    state = rooms.reducer(state, connected({ room: "r", live: false }))
    expect(state.r.connection).toBe("offline")
    state = rooms.reducer(state, removed("r"))
    state = rooms.reducer(state, connected({ room: "r", live: true }))
    expect(state.r.connection).toBe("removed")
  })

  it("keeps who is here and who is typing, and forgets the room when it is closed", () => {
    let state = rooms.reducer(undefined, connecting("r"))
    state = rooms.reducer(
      state,
      present({ room: "r", here: [{ user: "sam", name: "Sam", short: "S" }] })
    )
    state = rooms.reducer(
      state,
      typing({ room: "r", who: "sam", name: "Sam", until: 5 })
    )
    expect(state.r.here.map((each) => each.name)).toEqual(["Sam"])
    expect(state.r.typists.sam).toEqual({ name: "Sam", until: 5 })
    expect(rooms.reducer(state, closed("r")).r).toBeUndefined()
  })

  it("ignores news of a room that is not open", () => {
    expect(
      rooms.reducer(undefined, connected({ room: "x", live: true }))
    ).toEqual({})
  })
})
