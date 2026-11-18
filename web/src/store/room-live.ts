import type { ThunkDispatch, UnknownAction } from "@reduxjs/toolkit"

import { meaningOf, pending, putTurn } from "@/domain/live-room"
import { RoomSocket } from "@/lib/room-socket"
import type { LiveFrame, TurnsData } from "@/pages/conversation/types"
import { api } from "@/store/api"
import { rooms } from "@/store/rooms"
import { withBase } from "@/lib/base"

// RoomArg is the conversation whose turns are read: its page's address, and for a room with a live layer, where its stream
// stood when the page was read. The turns are kept by address alone, so reading the page again does not open a second socket.
export interface RoomArg {
  path: string
  room: string
  thread: string
  me: string
  stream: { Path: string; Seq: number; Epoch: string } | null
  // settings is the room's settings as the page was drawn with them; a snapshot that says otherwise reads the page again.
  settings: string
}

export const turnsPath = (path: string) => `/app/api${path}/turns`

// sockets are the open rooms' connections, by room, for what the page says and stops over them.
const sockets = new Map<string, RoomSocket>()

export const roomApi = api.injectEndpoints({
  endpoints: (build) => ({
    turns: build.query<TurnsData, RoomArg>({
      query: (arg) => turnsPath(arg.path),
      serializeQueryArgs: ({ queryArgs }) => queryArgs.path,
      providesTags: (_data, _error, arg) => [
        { type: "Page", id: turnsPath(arg.path) },
      ],
      // While a conversation is open, it is kept as it is now: over the room's socket when it has one, and by asking every few
      // seconds whether it moved on when it has not, or when the socket is down. A page showing a reply still being written
      // asks more often, since a socket can go quiet without saying so.
      async onCacheEntryAdded(
        arg,
        {
          dispatch,
          cacheDataLoaded,
          cacheEntryRemoved,
          updateCachedData,
          getCacheEntry,
        }
      ) {
        const snapshot = snapshotter(arg, dispatch, updateCachedData)
        let socket: RoomSocket | null = null
        try {
          await cacheDataLoaded
        } catch {
          return
        }
        if (arg.stream) {
          dispatch(rooms.actions.connecting(arg.room))
          socket = new RoomSocket(
            {
              room: arg.room,
              thread: arg.thread,
              me: arg.me,
              stream: arg.stream,
              versionPath: `${arg.path}/version`,
            },
            {
              onFrame: (frame) => {
                snapshot.apply(frame)
                socket?.seen()
              },
              onLost: () => void snapshot.take(),
              onOnline: (live) =>
                dispatch(rooms.actions.connected({ room: arg.room, live })),
              onRemoved: () => {
                dispatch(rooms.actions.removed(arg.room))
                window.location.assign(withBase("/chat"))
              },
              onPresence: (here) =>
                dispatch(rooms.actions.present({ room: arg.room, here })),
              onTyping: (who, name) =>
                dispatch(
                  rooms.actions.typing({
                    room: arg.room,
                    who,
                    name,
                    until: Date.now() + 5_000,
                  })
                ),
            }
          )
          sockets.set(arg.room, socket)
          socket.start()
        }
        const known = () => getCacheEntry().data?.Version
        const check = async () => {
          if (document.hidden) return
          const answer = await fetch(withBase(`${arg.path}/version`), {
            credentials: "same-origin",
            cache: "no-store",
          }).catch(() => null)
          if (answer?.ok && (await answer.text()) !== known())
            void snapshot.take()
        }
        let ticks = 0
        const every = window.setInterval(() => {
          ticks += 1
          const turns = getCacheEntry().data?.Turns ?? []
          // Without a live socket, every three seconds; with one, every ten while a reply is written and every thirty otherwise.
          const due = !socket?.online
            ? ticks % 3 === 0
            : pending(turns)
              ? ticks % 10 === 0
              : ticks % 30 === 0
          if (due) void check()
        }, 1_000)
        const back = () => {
          if (document.hidden) return
          void check()
          socket?.seen()
        }
        const leave = () => socket?.close()
        document.addEventListener("visibilitychange", back)
        window.addEventListener("online", back)
        window.addEventListener("pagehide", leave)
        await cacheEntryRemoved
        window.clearInterval(every)
        document.removeEventListener("visibilitychange", back)
        window.removeEventListener("online", back)
        window.removeEventListener("pagehide", leave)
        socket?.close()
        sockets.delete(arg.room)
        dispatch(rooms.actions.closed(arg.room))
      },
    }),
  }),
})

// snapshotter takes the whole conversation again. Frames that arrive while it is taken are held and put after it, since the
// snapshot may be older than they are.
function snapshotter(
  arg: RoomArg,
  dispatch: ThunkDispatch<unknown, unknown, UnknownAction>,
  updateCachedData: (recipe: (draft: TurnsData) => TurnsData | void) => void
) {
  let taking = false
  let held: LiveFrame[] = []
  const put = (frame: LiveFrame) => {
    const meaning = meaningOf(frame, arg.thread)
    if (meaning.kind === "reload") window.location.reload()
    else if (meaning.kind === "snapshot") void take()
    else if (meaning.kind === "put") {
      updateCachedData((draft) => {
        draft.Turns = putTurn(draft.Turns ?? [], meaning.turn)
        if (frame.ver) draft.Version = frame.ver
      })
    }
  }
  const take = async () => {
    if (taking) return
    taking = true
    try {
      const fresh = await dispatch(
        roomApi.endpoints.turns.initiate(arg, {
          forceRefetch: true,
          subscribe: false,
        })
      ).unwrap()
      // The room's settings are drawn with the page around the turns, so a change to them reads the page again.
      if (fresh.Settings && fresh.Settings !== arg.settings) {
        dispatch(
          api.util.invalidateTags([{ type: "Page", id: `/app/api${arg.path}` }])
        )
      }
    } catch {
      // The next check takes it again.
    } finally {
      taking = false
      const later = held
      held = []
      later.forEach(put)
    }
  }
  return {
    apply: (frame: LiveFrame) => (taking ? held.push(frame) : put(frame)),
    take,
  }
}

// say sends words to the room over its socket, and resends under the same id when the same words are sent again unanswered.
export function sayInRoom(room: string, text: string): Promise<void> {
  const socket = sockets.get(room)
  return socket
    ? socket.say(text)
    : Promise.reject(new Error("The room is not connected."))
}

// stopInRoom asks a reply to stop over the socket, and says whether the server did.
export function stopInRoom(room: string, ref: string): Promise<boolean> {
  return sockets.get(room)?.stop(ref) ?? Promise.resolve(false)
}

export function typedInRoom(room: string) {
  sockets.get(room)?.typing()
}
