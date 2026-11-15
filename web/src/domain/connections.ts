// What a person's Connections page decides from each app's row: what it says of their sign-in, and what they may do with it.

export interface Connection {
  ID: string
  Name: string
  What: string
  // Offered is an app an admin has turned on; one that is not is listed only while the person holds a sign-in to it.
  Offered: boolean
  Connected: boolean
  // SignInAgain is a sign-in the app no longer takes, which only the person can make again.
  SignInAgain: boolean
  ForAgents: boolean
  Since: number
}

export type Standing = "turned off" | "connected" | "sign in again" | "not yet"

// standingOf is what the page says of the person's sign-in to an app.
export function standingOf(app: Connection): Standing {
  if (!app.Offered) return "turned off"
  if (app.Connected) return "connected"
  if (app.SignInAgain) return "sign in again"
  return "not yet"
}

export interface ConnectionActions {
  // connect is the link to sign in at the app, by the words it is offered in, or nothing.
  connect?: "Connect" | "Sign in again"
  disconnect: boolean
  // forAgents is whether the person chooses here if their agents may use the app.
  forAgents: boolean
}

// actionsOf is what the person may do with an app: connect what they have not, sign in again where the app stopped taking their
// sign-in, and end any sign-in they hold, whether or not the app is still turned on. Taking back what one gave is always theirs.
export function actionsOf(app: Connection): ConnectionActions {
  const forAgents = app.Connected && app.Offered
  if (app.Connected || !app.Offered) return { disconnect: true, forAgents }
  if (app.SignInAgain)
    return { connect: "Sign in again", disconnect: true, forAgents }
  return { connect: "Connect", disconnect: false, forAgents }
}

// The addresses of an app's forms and links. Connecting is a link: the page it leads to is the app's.
export const connectPath = (id: string) =>
  `/connections/${encodeURIComponent(id)}/connect`
export const disconnectPath = (id: string) =>
  `/connections/${encodeURIComponent(id)}/disconnect`
export const forAgentsPath = (id: string) =>
  `/connections/${encodeURIComponent(id)}/agents`
