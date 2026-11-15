import { createSlice, type PayloadAction } from "@reduxjs/toolkit"

import type { Present } from "@/lib/room-socket"

// A room's live connection is in one of these states, and in no other: connecting before it first subscribes, live while its
// stream is subscribed, offline while it tries again, and removed once the person is taken out of the room or it is gone.
export type Connection = "connecting" | "live" | "offline" | "removed"

export interface RoomState {
  connection: Connection
  here: Present[]
  // typists are who is typing, and until when they are shown as typing.
  typists: Record<string, { name: string; until: number }>
}

const opened = (): RoomState => ({
  connection: "connecting",
  here: [],
  typists: {},
})

export const rooms = createSlice({
  name: "rooms",
  initialState: {} as Record<string, RoomState>,
  reducers: {
    connecting(state, action: PayloadAction<string>) {
      state[action.payload] = opened()
    },
    connected(state, action: PayloadAction<{ room: string; live: boolean }>) {
      const room = state[action.payload.room]
      if (room && room.connection !== "removed")
        room.connection = action.payload.live ? "live" : "offline"
    },
    removed(state, action: PayloadAction<string>) {
      const room = state[action.payload]
      if (room) room.connection = "removed"
    },
    present(state, action: PayloadAction<{ room: string; here: Present[] }>) {
      const room = state[action.payload.room]
      if (room) room.here = action.payload.here
    },
    typing(
      state,
      action: PayloadAction<{
        room: string
        who: string
        name: string
        until: number
      }>
    ) {
      const room = state[action.payload.room]
      if (room)
        room.typists[action.payload.who] = {
          name: action.payload.name,
          until: action.payload.until,
        }
    },
    closed(state, action: PayloadAction<string>) {
      delete state[action.payload]
    },
  },
})
