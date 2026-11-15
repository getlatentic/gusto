import { Centrifuge, type ClientInfo, type Subscription } from "centrifuge"

import type { LiveFrame } from "@/pages/conversation/types"

// A room, live. One WebSocket carries what others say and what agents do into the page as it happens, who is here and who is
// typing, and sends what the person says, stops and has read. It is Centrifuge's client over the server's /live.
//
// The stream is positioned: the page says where in the room's log what it shows was drawn, and the server replays what happened
// since, in order, so nothing is lost or shown twice across a reconnect or a restart. When it cannot, onLost asks for the whole
// conversation again.

export interface Present {
  user: string
  name: string
  short: string
}

export interface RoomEvents {
  onFrame: (frame: LiveFrame) => void
  onLost: () => void
  onOnline: (online: boolean) => void
  // onRemoved is the person taken out of the room, or the room gone.
  onRemoved: () => void
  onPresence: (people: Present[]) => void
  onTyping: (who: string, name: string) => void
}

export interface RoomPlace {
  room: string
  thread: string
  me: string
  stream: { Path: string; Seq: number; Epoch: string }
  // versionPath is asked when the socket keeps failing, to tell a person who signed out from a network that dropped.
  versionPath: string
}

interface Result {
  ok?: boolean
  error?: string
}

let counter = 0

// newID is a command's own id, so a command sent again after its answer was lost is done once.
function newID(): string {
  counter += 1
  const random = crypto.getRandomValues(new Uint32Array(2)).join("")
  return `c${Date.now().toString(36)}-${counter}-${random}`
}

export class RoomSocket {
  private readonly client: Centrifuge
  private readonly stream: Subscription
  private readonly channel: Subscription
  private readonly everyone = new Map<string, ClientInfo>()
  private failures = 0
  private sending: { id: string; text: string } | null = null
  private toldTypingAt = 0
  private readonly place: RoomPlace
  private readonly events: RoomEvents
  online = false

  constructor(place: RoomPlace, events: RoomEvents) {
    this.place = place
    this.events = events
    const scheme = location.protocol === "https:" ? "wss://" : "ws://"
    this.client = new Centrifuge(scheme + location.host + place.stream.Path, {
      minReconnectDelay: 500,
      maxReconnectDelay: 10_000,
      timeout: 8_000,
    })
    this.stream = this.client.newSubscription(`room:${place.room}`, {
      positioned: true,
      recoverable: true,
      since: { offset: place.stream.Seq, epoch: place.stream.Epoch },
    })
    this.channel = this.client.newSubscription(`live:${place.room}`, {
      joinLeave: true,
    })
    this.listen()
  }

  private setOnline(online: boolean) {
    this.online = online
    this.events.onOnline(online)
  }

  private listen() {
    this.stream.on("subscribed", (ctx) => {
      this.setOnline(true)
      if (ctx.wasRecovering && !ctx.recovered) this.events.onLost()
    })
    this.stream.on("subscribing", () => this.setOnline(false))
    this.stream.on("unsubscribed", (ctx) => {
      this.setOnline(false)
      if (ctx.code === 2403 || ctx.code === 103) this.events.onRemoved()
    })
    this.stream.on("publication", (ctx) =>
      this.events.onFrame(ctx.data as LiveFrame)
    )
    this.channel.on("subscribed", () => {
      this.channel
        .presence()
        .then((result) => {
          this.everyone.clear()
          for (const info of Object.values(result.clients)) this.arrive(info)
          this.tellPresence()
        })
        .catch(() => {})
    })
    this.channel.on("join", (ctx) => {
      this.arrive(ctx.info)
      this.tellPresence()
    })
    this.channel.on("leave", (ctx) => {
      this.everyone.delete(ctx.info.client)
      this.tellPresence()
    })
    this.channel.on("publication", (ctx) => {
      const data = ctx.data as {
        type?: string
        who?: string
        name?: string
        thread?: string
      }
      if (data?.type !== "typing" || !data.who || data.who === this.place.me)
        return
      if (data.thread && data.thread !== this.place.thread) return
      this.events.onTyping(data.who, data.name ?? "")
    })
    this.client.on("connected", () => {
      this.failures = 0
    })
    this.client.on("error", () => this.checkSignedIn())
  }

  // checkSignedIn stops trying, every third failure, when it is because the person signed out, which a failed upgrade cannot say.
  private checkSignedIn() {
    this.failures += 1
    if (this.failures % 3 !== 0) return
    fetch(this.place.versionPath, {
      credentials: "same-origin",
      redirect: "manual",
    })
      .then((answer) => {
        if (
          answer.type === "opaqueredirect" ||
          answer.status === 401 ||
          answer.status === 403
        ) {
          this.client.disconnect()
        }
      })
      .catch(() => {})
  }

  private arrive(info: ClientInfo | undefined) {
    if (!info?.user || info.user === this.place.me) return
    this.everyone.set(info.client, info)
  }

  // tellPresence gives one entry for each other person, however many tabs they have open.
  private tellPresence() {
    const people = new Map<string, Present>()
    for (const info of this.everyone.values()) {
      const detail = (info.connInfo ?? {}) as { name?: string; short?: string }
      people.set(info.user, {
        user: info.user,
        name: detail.name || "Someone",
        short: detail.short ?? "",
      })
    }
    this.events.onPresence([...people.values()])
  }

  start() {
    this.stream.subscribe()
    this.channel.subscribe()
    this.client.connect()
  }

  close() {
    this.client.disconnect()
  }

  private async call(
    method: string,
    body: object,
    id = newID(),
    retry = false
  ): Promise<Result> {
    const command = {
      v: 1,
      id,
      room: this.place.room,
      ...body,
      ...(retry ? { retry: true } : {}),
    }
    const reply = await this.client.rpc(method, command)
    return (reply.data ?? {}) as Result
  }

  // say sends a message, and resends under the same id when the same words are sent again before an answer came.
  async say(text: string): Promise<void> {
    const again = this.sending !== null && this.sending.text === text
    if (!again) this.sending = { id: newID(), text }
    const result = await this.call(
      "say",
      { thread: this.place.thread, text },
      this.sending!.id,
      again
    )
    if (!result.ok)
      throw new Error(result.error || "The server did not say why.")
    this.sending = null
  }

  // stop asks a reply to stop, and says whether the server did.
  async stop(ref: string): Promise<boolean> {
    try {
      return Boolean((await this.call("stop", { ref })).ok)
    } catch {
      return false
    }
  }

  seen() {
    if (!this.online) return
    this.call("seen", {
      thread: this.place.thread,
      at: Math.floor(Date.now() / 1000),
    }).catch(() => {})
  }

  // typing is told to the room at most every three seconds.
  typing() {
    const now = Date.now()
    if (!this.online || now - this.toldTypingAt < 3_000) return
    this.toldTypingAt = now
    this.call("typing", { thread: this.place.thread }).catch(() => {})
  }
}
