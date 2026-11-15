// The conversation page's data, as /app/api/instances/{id}/threads/{thread} gives it, and one turn as the page, the turns
// snapshot and a live frame all carry it. The server builds these in Go (conversation_view.go); the two must stay the same shape.

import type { Option } from "@/domain/chat-choices"

// FileChip is one file a reply produced: Look is the icon it draws (image, code, doc or blank); Gone says why a file has no chip.
export interface FileChip {
  Path: string
  Name: string
  Look: string
  Gone: string
}

export interface TurnFiles {
  Shown: FileChip[] | null
  More: FileChip[] | null
  Removed: FileChip[] | null
}

// TurnView is one turn of a conversation, or one note or notice of a room, in its place among the others.
export interface TurnView {
  ID: string
  // Kind is "turn" for a message and its reply, "note" for a room message nobody was asked to answer, and "notice" for what
  // the room says itself (someone joined, a file was attached).
  Kind: "turn" | "note" | "notice"
  // Divider is the day or time drawn above this turn when it starts a new stretch, or nothing.
  Divider: string

  // The message. ShowSaid is false when an agent asked it of another, which shows only the reply. SaidHTML is Said with each
  // mention of someone in the room marked as <span class="mention">@Name</span>, and nothing else marked; the server escapes
  // the rest. SaidJoins draws it under the message before, by the same person, without a name of its own.
  Who: string
  WhoName: string
  WhoInitials: string
  Said: string
  SaidHTML: string
  ShowSaid: boolean
  SaidJoins: boolean
  Queued: number

  // The reply. By is the agent's name, Owner whose agent it is (an address; nothing for the conversation's own agent), From
  // which agent asked it (a room's agent asking another).
  By: string
  Role: string
  Harness: string
  Mark: string
  MarkDark: string
  Owner: string
  // OwnerName is the owner as the room names them, for "Ada's agent"; the page says "your agent" for the viewer's own.
  OwnerName: string
  From: string
  ReplyJoins: boolean
  Done: boolean
  // Holding is why a reply waits to start, when it does.
  Holding: string
  Started: number
  Ended: number
  // Took is how long the reply took and Money what it cost, when it is done and priced, as the page says them.
  Took: string
  Money: string
  Steps: { What: string; Detail: string }[] | null
  // Apps is the apps the agent could use, in words, when it had any.
  Apps: string
  // ReplyHTML is the reply's markdown drawn as HTML by the server, with its links to files rewritten to open them here.
  ReplyHTML: string
  // Capped is the spending limit that stopped the reply: Credit says it is gusto's free credit, used up.
  Capped: {
    Credit: boolean
    SpentUSD: number
    LimitUSD: number
    Words: string
  } | null
  StoppedBy: string
  Stopped: string
  Home: { Outcome: string; Used: string; Limit: string } | null
  Blocked: number
  Refused: { Host: string; Times: number }[] | null
  Previews: string[] | null
  Files: TurnFiles | null
  // File is the file a notice points at, when it does.
  File: string
}

export interface RoomPerson {
  Who: string
  Name: string
  Role: string
  Me: boolean
}

export interface RoomAgent {
  ID: string
  Name: string
  Harness: string
  Mark: string
  MarkDark: string
  // Engine is the agent's type, Brain its model, and Job what it is for, when it says.
  Engine: string
  Brain: string
  Job: string
  // Owner is whose agent it is, as the dialog names them; nothing for the viewer's own.
  Owner: string
  Pending: boolean
  // Allowance is what it may spend here a month, in words; AllowanceField the same as its field holds it.
  Allowance: string
  AllowanceField: string
  Mine: boolean
}

// Mentionable is someone a message can mention with @: an agent or a person of the room.
export interface Mentionable {
  name: string
  role: string
  owner: string
  agent: boolean
  icon: string
  short: string
}

export interface ConversationData {
  Instance: {
    ID: string
    Title: string
    Room: boolean
    Direct: boolean
    Sensitivity: string
    Model: string
  }
  Thread: { ID: string; Dir: string }
  // Named is the conversation's whole title, which the page bar truncates to fit.
  Named: string
  Path: string
  Me: string
  Agent: {
    Name: string
    Harness: string
    Brain: string
    Mark: string
    MarkDark: string
  }
  // DirectAgent is the one agent of a person's own room with one agent, which the page is named after.
  DirectAgent: {
    Name: string
    Harness: string
    Mark: string
    MarkDark: string
  } | null
  Turns: TurnView[] | null
  Starters: string[] | null
  // Several says the model can be changed here, and Models what it can be changed to.
  Several: boolean
  Models: Option[] | null
  Sharing: boolean
  Owns: boolean
  Apps: { Name: string; Link: string }[] | null
  Starting: boolean
  // Waiting says a conversation (not a room) has a reply running; Latest is its task, and Holding why it has not started.
  Waiting: boolean
  Latest: string
  Holding: string
  Hint: string
  Mentions: Mentionable[] | null
  Names: string[] | null
  // Stream is where the room's live layer is, from which point; nothing when the room is not live.
  Stream: { Path: string; Seq: number; Epoch: string } | null
  Version: string
  Headcount: number
  Settings: {
    Archived: boolean
    Runs: boolean
    In: boolean
    Muted: boolean
    Mark: string
  }
  Members: {
    People: RoomPerson[] | null
    Agents: RoomAgent[] | null
    Runs: boolean
    Addable:
      | {
          ID: string
          Name: string
          Harness: string
          Mark: string
          MarkDark: string
          Engine: string
          Specialty: string
        }[]
      | null
    // RoomAgents is how many agents the room has, for what the add dialog says when there are none to add.
    RoomAgents: number
  } | null
  Guidelines: string
  MaxGuidelines: number
  Refused: string
  AddRefused: string
  GuidelinesRefused: string
  Unsent: string
  // AttachMost and AttachBytes are how many files a room message may carry, and how big each may be.
  AttachMost: number
  AttachBytes: number
}

// TurnsData is /app/api/instances/{id}/threads/{thread}/turns: the turns as they are now, for a snapshot.
export interface TurnsData {
  Turns: TurnView[] | null
  Version: string
  Settings: string
}

// LiveFrame is one publication on a room's channel. A turn or note frame carries the turn; people and refresh frames say to
// take a snapshot; v other than 1 says the page is out of date.
export interface LiveFrame {
  v: number
  type: "turn" | "note" | "people" | "refresh" | "skip"
  room: string
  thread: string
  seq: number
  id: string
  turn?: TurnView
  ver: string
}

export interface RoomFile {
  Path: string
  Name: string
  Folder: string
  Size: number
  Modified: number
}

// RoomFilesData is /app/api/rooms/{id}/files: a room's files by who made them, newest day first.
export interface RoomFilesData {
  Room: { ID: string; Title: string }
  From: string
  ByAgents: { Day: string; Files: RoomFile[] }[] | null
  ByPeople: { Day: string; Files: RoomFile[] }[] | null
  Agents: number
  People: number
  Cut: boolean
  Archived: boolean
}
