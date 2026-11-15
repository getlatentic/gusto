import type { ConversationData, TurnView } from "@/pages/conversation/types"

export function turnOf(id: string, change: Partial<TurnView> = {}): TurnView {
  return {
    ID: id,
    Kind: "turn",
    Divider: "",
    Who: "ada@acme.com",
    WhoName: "Ada",
    WhoInitials: "A",
    Said: "Draft the policy",
    SaidHTML: "Draft the policy",
    ShowSaid: true,
    SaidJoins: false,
    Queued: 1_700_000_000,
    By: "Scout",
    Role: "",
    Harness: "codex",
    Mark: "",
    MarkDark: "",
    Owner: "",
    OwnerName: "",
    From: "",
    ReplyJoins: false,
    Done: true,
    Holding: "",
    Started: 0,
    Ended: 0,
    Took: "12s",
    Money: "$0.02",
    Steps: null,
    Apps: "",
    ReplyHTML: "<p>Here it is.</p>",
    Capped: null,
    StoppedBy: "",
    Stopped: "",
    Home: null,
    Blocked: 0,
    Refused: null,
    Previews: null,
    Files: null,
    File: "",
    ...change,
  }
}

export function conversationOf(
  change: (page: ConversationData) => void = () => {}
): ConversationData {
  const page: ConversationData = {
    Instance: {
      ID: "a",
      Title: "Policy",
      Room: false,
      Direct: false,
      Sensitivity: "",
      Model: "m1",
    },
    Thread: { ID: "1", Dir: "docs" },
    Named:
      "Draft a one-page policy for how we use AI at work, with a section on what to keep private",
    Path: "/instances/a/threads/1",
    Me: "ada@acme.com",
    Agent: {
      Name: "Scout",
      Harness: "codex",
      Brain: "GPT",
      Mark: "",
      MarkDark: "",
    },
    DirectAgent: null,
    Turns: [turnOf("t1")],
    Starters: null,
    Several: false,
    Models: null,
    Sharing: false,
    Owns: true,
    Apps: null,
    Starting: false,
    Waiting: false,
    Latest: "",
    Holding: "",
    Hint: "Message the room",
    Mentions: null,
    Names: null,
    Stream: null,
    Version: "1",
    Headcount: 2,
    Settings: {
      Archived: false,
      Runs: true,
      In: true,
      Muted: false,
      Mark: "s",
    },
    Members: null,
    Guidelines: "",
    MaxGuidelines: 2000,
    Refused: "",
    AddRefused: "",
    GuidelinesRefused: "",
    Unsent: "",
    AttachMost: 2,
    AttachBytes: 100,
  }
  change(page)
  return page
}

// roomOf is a room with people and agents, where @ mentions them.
export function roomOf(
  change: (page: ConversationData) => void = () => {}
): ConversationData {
  return conversationOf((page) => {
    page.Instance = { ...page.Instance, Room: true, Title: "Launch" }
    page.Named = "Launch"
    page.Mentions = [
      { name: "Sam", role: "", owner: "", agent: false, icon: "", short: "S" },
      {
        name: "Scout",
        role: "Researcher",
        owner: "your agent",
        agent: true,
        icon: "",
        short: "",
      },
    ]
    page.Names = ["Sam", "Scout"]
    page.Members = {
      People: [{ Who: "ada@acme.com", Name: "Ada", Role: "admin", Me: true }],
      Agents: null,
      Runs: true,
      Addable: null,
      RoomAgents: 1,
    }
    change(page)
  })
}
