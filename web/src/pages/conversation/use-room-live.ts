import { useEffect, useEffectEvent, useRef, useState } from "react"
import {
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"

import { meaningOf, pending, putTurn } from "@/domain/live-room"
import { getJSON } from "@/lib/api"
import { RoomSocket, type Present } from "@/lib/room-socket"
import type {
  ConversationData,
  LiveFrame,
  TurnsData,
} from "@/pages/conversation/types"

// The conversation's turns live in their own query, so a live frame can put one turn in place without fetching the page.
export const turnsPath = (page: ConversationData) =>
  `/app/api${page.Path}/turns`

export interface RoomLive {
  turns: TurnsData
  // online says the live socket is up, so a message of words alone goes over it.
  online: boolean
  here: Present[]
  typing: string[]
  say: (text: string) => Promise<void>
  // stop asks a reply to stop over the socket, and says whether it did; the page posts the stop form when it did not.
  stop: (ref: string) => Promise<boolean>
  typed: () => void
}

interface Typist {
  name: string
  until: number
}

// snapshotter fetches the whole conversation again. Frames that arrive while it is fetched are held and put after it, since
// the snapshot may be older than they are.
function snapshotter(
  client: QueryClient,
  key: string,
  thread: string,
  onSettings: (mark: string) => void
) {
  let fetching = false
  let held: LiveFrame[] = []
  const put = (frame: LiveFrame) => {
    const meaning = meaningOf(frame, thread)
    if (meaning.kind === "reload") window.location.reload()
    else if (meaning.kind === "snapshot") void snapshot()
    else if (meaning.kind === "put") {
      client.setQueryData<TurnsData>([key], (now) =>
        now
          ? {
              ...now,
              Turns: putTurn(now.Turns ?? [], meaning.turn),
              Version: frame.ver || now.Version,
            }
          : now
      )
    }
  }
  const snapshot = async () => {
    if (fetching) return
    fetching = true
    try {
      const fresh = await client.fetchQuery({
        queryKey: [key],
        queryFn: () => getJSON<TurnsData>(key),
        staleTime: 0,
      })
      onSettings(fresh.Settings)
    } catch {
      // The next check tries again.
    } finally {
      fetching = false
      const later = held
      held = []
      later.forEach(put)
    }
  }
  const apply = (frame: LiveFrame) => (fetching ? held.push(frame) : put(frame))
  return { apply, snapshot }
}

// useRoomLive keeps a conversation as it is now: over the room's live socket when it has one, and by asking every few seconds
// whether it moved on when it has not, or when the socket is down. A page that shows a reply still being written asks more
// often, since a socket can go quiet without saying so.
export function useRoomLive(page: ConversationData): RoomLive {
  const key = turnsPath(page)
  const client = useQueryClient()
  const turns = useQuery({
    queryKey: [key],
    queryFn: ({ signal }) => getJSON<TurnsData>(key, signal),
    initialData: {
      Turns: page.Turns,
      Version: page.Version,
      Settings: page.Settings.Mark,
    },
    staleTime: Infinity,
  })
  const [online, setOnline] = useState(false)
  const [here, setHere] = useState<Present[]>([])
  const [typists, setTypists] = useState<Record<string, Typist>>({})
  const [now, setNow] = useState(() => Date.now())
  const socket = useRef<RoomSocket | null>(null)
  // A change to the room's settings (archived, guidelines) changes the page around the turns, which is fetched again.
  const settingsMoved = useEffectEvent((mark: string) => {
    if (mark && mark !== page.Settings.Mark)
      void client.invalidateQueries({ queryKey: [`/app/api${page.Path}`] })
  })
  const versionPath = `${page.Path}/version`

  useEffect(() => {
    const live = snapshotter(client, key, page.Thread.ID, (mark) =>
      settingsMoved(mark)
    )
    let opened: RoomSocket | null = null
    if (page.Stream) {
      opened = new RoomSocket(
        {
          room: page.Instance.ID,
          thread: page.Thread.ID,
          me: page.Me,
          stream: page.Stream,
          versionPath,
        },
        {
          onFrame: (frame) => {
            live.apply(frame)
            opened?.seen()
          },
          onLost: () => void live.snapshot(),
          onOnline: setOnline,
          onRemoved: () => window.location.assign("/chat"),
          onPresence: setHere,
          onTyping: (who, name) =>
            setTypists((all) => ({
              ...all,
              [who]: { name, until: Date.now() + 5_000 },
            })),
        }
      )
      socket.current = opened
      opened.start()
    }
    const check = async () => {
      if (document.hidden) return
      const known = client.getQueryData<TurnsData>([key])?.Version
      const answer = await fetch(versionPath, {
        credentials: "same-origin",
        cache: "no-store",
      }).catch(() => null)
      if (answer?.ok && (await answer.text()) !== known) void live.snapshot()
    }
    let ticks = 0
    const every = window.setInterval(() => {
      ticks += 1
      setNow(Date.now())
      const shown = client.getQueryData<TurnsData>([key])?.Turns ?? []
      const quiet = opened?.online
      // Without a live socket, every three seconds; with one, every ten while a reply is written and every thirty otherwise.
      if (
        !quiet
          ? ticks % 3 === 0
          : pending(shown)
            ? ticks % 10 === 0
            : ticks % 30 === 0
      )
        void check()
    }, 1_000)
    const back = () => {
      if (document.hidden) return
      void check()
      opened?.seen()
    }
    document.addEventListener("visibilitychange", back)
    window.addEventListener("online", back)
    window.addEventListener("pagehide", () => opened?.close())
    return () => {
      window.clearInterval(every)
      document.removeEventListener("visibilitychange", back)
      window.removeEventListener("online", back)
      opened?.close()
      socket.current = null
    }
  }, [
    client,
    key,
    page.Instance.ID,
    page.Thread.ID,
    page.Me,
    page.Stream,
    versionPath,
  ])

  // Someone typing is shown for five seconds after they last were, and not once their message is in.
  const typing = Object.values(typists)
    .filter((typist) => typist.until > now)
    .map((typist) => typist.name)
  return {
    turns: turns.data,
    online,
    here,
    typing,
    say: (text) =>
      socket.current?.say(text) ??
      Promise.reject(new Error("The room is not connected.")),
    stop: (ref) => socket.current?.stop(ref) ?? Promise.resolve(false),
    typed: () => socket.current?.typing(),
  }
}
