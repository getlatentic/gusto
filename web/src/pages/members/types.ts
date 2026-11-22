// The members pages' data: the list and its dialogs as /app/api/members gives it, and whoever is open as
// /app/api/members/agents/{id} and /app/api/members/people/{who} give them. The server builds these in Go (members.go and
// members_pages.go); the two must stay the same shape.

// ListedPerson is somebody in the team. SharedSignIn is the server's one shared sign-in, which has no email address.
export interface ListedPerson {
  Who: string
  Name: string
  Initials: string
  Role: string
  Where: string
  SharedSignIn: boolean
}

// ListedAgent is an agent of the team: Engine is its agent type by name, and Mark that type's logo.
export interface ListedAgent {
  ID: string
  Name: string
  Harness: string
  Mark: string
  MarkDark: string
  Engine: string
  Specialty: string
  Where: string
}

// Kind is an agent type the new agent dialog offers. Installable is one gusto can install on the person's own computer.
export interface Kind {
  Name: string
  Title: string
  What: string
  Why: string
  Mark: string
  MarkDark: string
  Ready: boolean
  Installable: boolean
}

export interface ModelOption {
  value: string
  label: string
  detail?: string
}

export interface NewAgent {
  Kinds: Kind[]
  // FirstReady is the agent type chosen first, and nothing when none can run.
  FirstReady: string
  Offered: ModelOption[]
}

// Invite is gusto cloud's form for inviting a person: where it is posted, the team they join, and this page, to come back to.
export interface Invite {
  Action: string
  Team: string
  Back: string
}

// MembersData is the list and the dialogs that add to it, which every members page shows.
export interface MembersData {
  People: ListedPerson[]
  Agents: ListedAgent[]
  // Keeps is whether this server keeps agents at all.
  Keeps: boolean
  NewAgent: NewAgent
  // Invite is offered only to an admin of an organization behind gusto cloud.
  Invite: Invite | null
  // Sent is gusto cloud's word that the invitation went.
  Sent: boolean
  Refused: string
}

export interface Access {
  Who: string
  Name: string
  Initials: string
}

// AgentPage is the agent an agent's page is of. Mine is whether the person looking may change it, and Usable whether they may
// work with it. Brain is its model by name, and Model as its field holds it, empty for its default.
export interface AgentPage extends ListedAgent {
  Brain: string
  Model: string
  Persona: string
  Setup: string
  Owner: string
  OwnerName: string
  OwnerInitials: string
  Access: Access[]
  Created: number
  Mine: boolean
  Usable: boolean
}

// AgentApp is one app of the person looking: what they chose for this agent, and their default, which inherit follows. An app
// whose sign-in no longer works has nothing to choose until they sign in again.
export interface AgentApp {
  ID: string
  Name: string
  Choice: "inherit" | "allow" | "deny"
  Default: boolean
  SignInAgain: boolean
}

// AgentApps is the Apps tab: Person is false for the shared sign-in, which has no apps, and Works whether the person looking
// works with the agent, since only they choose for it.
export interface AgentApps {
  Apps: AgentApp[] | null
  Person: boolean
  Works: boolean
}

export interface SkillChoice {
  Name: string
  Title: string
  // Role is one of the ready-made roles, and otherwise a skill set made for the team.
  Role: boolean
}

// AgentSkills is the Skills tab: what the agent can be given, and the skill set it has, by Title, with what it holds.
export interface AgentSkills {
  Choices: SkillChoice[] | null
  Given: boolean
  Title: string
  Skills: { Name: string; Description: string }[]
}

export interface RoomLink {
  Name: string
  Where: string
}

// AgentData is an agent's page. Refused is what a form sent to it was refused for.
export interface AgentData {
  Agent: AgentPage
  // Apps is null where this server offers no apps.
  Apps: AgentApps | null
  Skills: AgentSkills
  Rooms: RoomLink[]
  Refused: string
}

export interface Bio {
  Text: string
  Mine: boolean
  Max: number
}

// PersonData is a person's page. Refused is what a form sent to it was refused for.
export interface PersonData {
  Person: ListedPerson
  Bio: Bio
  Rooms: RoomLink[]
  Made: ListedAgent[]
  Refused: string
}
