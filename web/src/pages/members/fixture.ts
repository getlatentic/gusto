import type {
  AgentData,
  AgentPage,
  Kind,
  ListedAgent,
  ListedPerson,
  MembersData,
  PersonData,
} from "@/pages/members/types"

export const ada: ListedPerson = {
  Who: "ada@acme.com",
  Name: "Ada",
  Initials: "A",
  Role: "Admin",
  Where: "/members/people/ada@acme.com",
  SharedSignIn: false,
}

export const bob: ListedPerson = {
  Who: "bob@acme.com",
  Name: "Bob",
  Initials: "B",
  Role: "Member",
  Where: "/members/people/bob@acme.com",
  SharedSignIn: false,
}

export const atlas: ListedAgent = {
  ID: "a1",
  Name: "Atlas",
  Harness: "codex",
  Mark: "/static/agents/codex.svg",
  MarkDark: "",
  Engine: "Codex",
  Specialty: "Research",
  Where: "/members/agents/a1",
}

export function kindOf(change: Partial<Kind> = {}): Kind {
  return {
    Name: "codex",
    Title: "Codex",
    What: "Writes and runs code",
    Why: "",
    Mark: "/static/agents/codex.svg",
    MarkDark: "",
    Ready: true,
    Installable: false,
    ...change,
  }
}

export function membersOf(
  change: (page: MembersData) => void = () => {}
): MembersData {
  const page: MembersData = {
    People: [ada, bob],
    Agents: [atlas],
    Keeps: true,
    NewAgent: {
      Kinds: [kindOf()],
      FirstReady: "codex",
      Offered: [{ value: "acme/big-1", label: "big-1" }],
    },
    Invite: null,
    Sent: false,
    Refused: "",
  }
  change(page)
  return page
}

export function agentOf(change: Partial<AgentPage> = {}): AgentPage {
  return {
    ...atlas,
    Brain: "big-1",
    Model: "acme/big-1",
    Persona: "You are careful.",
    Setup: "",
    Owner: ada.Who,
    OwnerName: ada.Name,
    OwnerInitials: ada.Initials,
    Access: [{ Who: bob.Who, Name: bob.Name, Initials: bob.Initials }],
    Created: Math.floor(Date.now() / 1000) - 120,
    Mine: true,
    Usable: true,
    ...change,
  }
}

export function agentPageOf(
  change: (page: AgentData) => void = () => {}
): AgentData {
  const page: AgentData = {
    Agent: agentOf(),
    Apps: null,
    Skills: {
      Choices: [
        { Name: "health", Title: "Health", Role: true },
        { Name: "launch", Title: "launch", Role: false },
      ],
      Given: false,
      Title: "",
      Skills: [],
    },
    Rooms: [{ Name: "Launch", Where: "/instances/r1/threads/t1" }],
    Refused: "",
  }
  change(page)
  return page
}

export function personPageOf(
  change: (page: PersonData) => void = () => {}
): PersonData {
  const page: PersonData = {
    Person: ada,
    Bio: { Text: "I run launches.", Mine: true, Max: 1000 },
    Rooms: [{ Name: "Launch", Where: "/instances/r1/threads/t1" }],
    Made: [atlas],
    Refused: "",
  }
  change(page)
  return page
}
