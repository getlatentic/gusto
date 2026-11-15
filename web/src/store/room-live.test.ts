import { afterEach, describe, expect, it, vi } from "vitest"

import { turnOf } from "@/pages/conversation/fixture"
import type { LiveFrame, TurnsData } from "@/pages/conversation/types"
import { makeStore } from "@/store"
import { api } from "@/store/api"
import { roomApi, type RoomArg } from "@/store/room-live"

// A stand-in for the room's socket, which the test drives as the server would.
const opened: {
  events: Record<string, (...args: never[]) => void>
  closed: boolean
}[] = []
vi.mock("@/lib/room-socket", () => ({
  RoomSocket: class {
    online = true
    events: Record<string, (...args: never[]) => void>
    closed = false
    constructor(
      _place: unknown,
      events: Record<string, (...args: never[]) => void>
    ) {
      this.events = events
      opened.push(this)
    }
    start() {}
    seen() {}
    close() {
      this.closed = true
    }
  },
}))

const arg: RoomArg = {
  path: "/instances/r/threads/1",
  room: "r",
  thread: "1",
  me: "ada@acme.com",
  stream: { Path: "/live?room=r", Seq: 3, Epoch: "e" },
  settings: "s",
}

const snapshot: TurnsData = {
  Turns: [turnOf("t1")],
  Version: "v1",
  Settings: "s",
}

const frame = (change: Partial<LiveFrame>): LiveFrame => ({
  v: 1,
  type: "turn",
  room: "r",
  thread: "1",
  seq: 4,
  id: "t2",
  ver: "v2",
  ...change,
})

afterEach(() => {
  opened.length = 0
  vi.unstubAllGlobals()
})

describe("a room kept live in the store", () => {
  it("opens the socket when its turns are read, puts each frame in place, and says when it is live", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify(snapshot)))
    )
    const store = makeStore()
    const read = store.dispatch(roomApi.endpoints.turns.initiate(arg))
    await read
    await vi.waitFor(() => expect(opened).toHaveLength(1))
    const socket = opened[0]
    socket.events.onOnline(true as never)
    expect(store.getState().rooms.r.connection).toBe("live")
    socket.events.onFrame(
      frame({ turn: turnOf("t2", { Done: false }) }) as never
    )
    socket.events.onFrame(frame({ turn: turnOf("t2"), ver: "v3" }) as never)
    const turns = roomApi.endpoints.turns.select(arg)(store.getState()).data!
    expect(turns.Turns!.map((turn) => [turn.ID, turn.Done])).toEqual([
      ["t1", true],
      ["t2", true],
    ])
    expect(turns.Version).toBe("v3")
    read.unsubscribe()
  })

  it("holds frames while it takes a snapshot, and puts them after it", async () => {
    let answer!: (response: Response) => void
    const fetched = vi.fn(async () => new Response(JSON.stringify(snapshot)))
    vi.stubGlobal("fetch", fetched)
    const store = makeStore()
    const read = store.dispatch(roomApi.endpoints.turns.initiate(arg))
    await read
    await vi.waitFor(() => expect(opened).toHaveLength(1))
    fetched.mockImplementationOnce(
      () => new Promise<Response>((resolve) => (answer = resolve))
    )
    opened[0].events.onLost()
    opened[0].events.onFrame(frame({ turn: turnOf("t3") }) as never)
    expect(
      roomApi.endpoints.turns
        .select(arg)(store.getState())
        .data!.Turns!.map((turn) => turn.ID)
    ).toEqual(["t1"])
    answer(
      new Response(
        JSON.stringify({ ...snapshot, Turns: [turnOf("t1"), turnOf("t2")] })
      )
    )
    await vi.waitFor(() =>
      expect(
        roomApi.endpoints.turns
          .select(arg)(store.getState())
          .data!.Turns!.map((turn) => turn.ID)
      ).toEqual(["t1", "t2", "t3"])
    )
    read.unsubscribe()
  })

  it("closes the socket and forgets the room when its turns leave the store", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify(snapshot)))
    )
    const store = makeStore()
    const read = store.dispatch(roomApi.endpoints.turns.initiate(arg))
    await read
    await vi.waitFor(() => expect(opened).toHaveLength(1))
    read.unsubscribe()
    store.dispatch(api.util.resetApiState())
    await vi.waitFor(() => expect(opened[0].closed).toBe(true))
    expect(store.getState().rooms.r).toBeUndefined()
  })
})
