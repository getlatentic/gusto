// The data of a person's account pages, as internal/web builds it: connections_data.go and device.go.

import type { Connection } from "@/domain/connections"

export interface ConnectionsData {
  // Person is whether whoever is signed in is a person, and not the shared sign-in, which has no accounts of its own.
  Person: boolean
  Connectors: Connection[]
  Refused: string
  // Done is the app the person has just signed in to.
  Done: string
  Home: KeptHome | null
  HomeReset: boolean
}

// KeptHome is the home a person's agents keep between tasks. Until a task of theirs has ended in it, nothing is counted.
export interface KeptHome {
  Counted: boolean
  Used: string
  Limit: string
  Full: boolean
  At: number
}

export interface DeviceData {
  // Code is the code the computer sent the person here with.
  Code: string
  Refused: string
  // Approved is the computer just approved.
  Approved: string
  // Shared is the shared sign-in, which is nobody and has no computers; Owner is the shared sign-in that is this server's owner.
  Shared: boolean
  Owner: boolean
  Tokens: ApprovedComputer[]
}

export interface ApprovedComputer {
  ID: string
  Name: string
  Made: string
  LastUsed: string
  Ended: string
  Live: boolean
}
