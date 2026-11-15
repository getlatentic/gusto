import { describe, expect, it } from "vitest"

import {
  actionsOf,
  connectPath,
  type Connection,
  standingOf,
} from "@/domain/connections"

const app = (change: Partial<Connection>): Connection => ({
  ID: "linear",
  Name: "Linear",
  What: "Issues",
  Offered: true,
  Connected: false,
  SignInAgain: false,
  ForAgents: false,
  Since: 0,
  ...change,
})

describe("an app on Connections", () => {
  it("is offered to connect while the person has not", () => {
    const fresh = app({})
    expect(standingOf(fresh)).toBe("not yet")
    expect(actionsOf(fresh)).toEqual({
      connect: "Connect",
      disconnect: false,
      forAgents: false,
    })
  })

  it("once connected, is ended from here and chosen for the person's agents", () => {
    const on = app({ Connected: true, Since: 1 })
    expect(standingOf(on)).toBe("connected")
    expect(actionsOf(on)).toEqual({ disconnect: true, forAgents: true })
  })

  it("whose sign-in no longer works, is signed in to again or ended, and not given to agents", () => {
    const stale = app({ SignInAgain: true })
    expect(standingOf(stale)).toBe("sign in again")
    expect(actionsOf(stale)).toEqual({
      connect: "Sign in again",
      disconnect: true,
      forAgents: false,
    })
  })

  it("that an admin turned off, is only ended", () => {
    for (const off of [
      app({ Offered: false, Connected: true }),
      app({ Offered: false, SignInAgain: true }),
    ]) {
      expect(standingOf(off)).toBe("turned off")
      expect(actionsOf(off)).toEqual({ disconnect: true, forAgents: false })
    }
  })

  it("is connected at its own address", () => {
    expect(connectPath("my app")).toBe("/connections/my%20app/connect")
  })
})
