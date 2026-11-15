import { useEffect, useState } from "react"

import type { Present } from "@/lib/room-socket"
import type { ConversationData, TurnsData } from "@/pages/conversation/types"
import { useAppSelector } from "@/store"
import { roomApi, sayInRoom, stopInRoom, typedInRoom } from "@/store/room-live"

export interface RoomLive {
  turns: TurnsData
  // online says the live socket is up, so a message of words alone goes over it.
  online: boolean
  here: Present[]
  typing: string[]
  say: (text: string) => Promise<void>
  // stop asks a reply to stop over the socket, and says whether it did; the page sends the stop form when it did not.
  stop: (ref: string) => Promise<boolean>
  typed: () => void
}

const nobody: Record<string, { name: string; until: number }> = {}

// useRoomLive is a conversation as it is now. Reading its turns opens the room's live connection (see store/room-live.ts),
// which keeps them current and is closed when no page reads them.
export function useRoomLive(page: ConversationData): RoomLive {
  const room = page.Instance.ID
  const read = roomApi.endpoints.turns.useQuery(
    {
      path: page.Path,
      room,
      thread: page.Thread.ID,
      me: page.Me,
      stream: page.Stream,
      settings: page.Settings.Mark,
    },
    // The turns are kept current by the room itself, which checks again when the person comes back; a refetch on focus could
    // land after a frame and put back an older list.
    { refetchOnFocus: false }
  )
  const state = useAppSelector((all) => all.rooms[room])
  const typists = state?.typists ?? nobody
  const [now, setNow] = useState(() => Date.now())
  const someone = Object.keys(typists).length > 0
  // Someone typing is shown for five seconds after they last were, so the time is followed while anyone has been typing.
  useEffect(() => {
    if (!someone) return
    const tick = window.setInterval(() => setNow(Date.now()), 1_000)
    return () => window.clearInterval(tick)
  }, [someone])
  return {
    turns: read.data ?? {
      Turns: page.Turns,
      Version: page.Version,
      Settings: page.Settings.Mark,
    },
    online: state?.connection === "live",
    here: state?.here ?? [],
    typing: Object.values(typists)
      .filter((typist) => typist.until > now)
      .map((typist) => typist.name),
    say: (text) => sayInRoom(room, text),
    stop: (ref) => stopInRoom(room, ref),
    typed: () => typedInRoom(room),
  }
}
