// The side list beside a person's conversations, as the server gives it, and what the list makes of it. Nothing here draws.

export interface Unread {
  Count: number
  ForYou: number
}

// RailChat is one conversation in the list. A room has an ID; a person's own room with one agent shows as that agent.
export interface RailChat {
  Name: string
  Where: string
  When: number
  Room: boolean
  Unread: Unread
  ID: string
  Harness: string
  Mark: string
  MarkDark: string
}

// RailGroup is one project: an agent's folder, with its latest conversations.
export interface RailGroup {
  ID: string
  Name: string
  Detail: string
  Runs: string
  Chats: RailChat[] | null
}

export interface RailList {
  Direct: RailChat[] | null
  Rooms: RailChat[] | null
  Archived: RailChat[] | null
  Latest: RailChat[] | null
  Unread: number
  ForYou: number
  Recent: RailGroup[] | null
  Older: RailGroup[] | null
}

export interface Workspace {
  ID: string
  Name: string
}

export interface PersonShell {
  Rail: RailList
  SignedAs: { Name: string; Initials: string; Who: string }
  Admin: boolean
  Connections: boolean
  Org: {
    Slug: string
    Name: string
    Manage: string
    OrgsFrom: string
    AccountFrom: string
    ConfirmAgain: string
  } | null
  Workspace: string
  Workspaces: { Here: Workspace; Other: Workspace[] | null } | null
}

export type SideTab = "home" | "recent" | "unread" | "me"

export const sideTabs: SideTab[] = ["home", "recent", "unread", "me"]

export function isSideTab(name: string | null): name is SideTab {
  return sideTabs.includes(name as SideTab)
}

// rooms are every room the person is in, their own with one agent first, as the counts of the unread tabs count them.
function rooms(rail: RailList): RailChat[] {
  return [...(rail.Direct ?? []), ...(rail.Rooms ?? [])]
}

// unreadRooms are the rooms with something the person has not read.
export function unreadRooms(rail: RailList): RailChat[] {
  return rooms(rail).filter((room) => room.Unread.Count > 0)
}

// roomsForYou are the rooms where something unread mentions the person.
export function roomsForYou(rail: RailList): RailChat[] {
  return rooms(rail).filter((room) => room.Unread.ForYou > 0)
}

// isOpen is whether a project shows its conversations: the first does, and so does the one holding the conversation open.
export function isOpen(group: RailGroup, index: number, here: string): boolean {
  return index === 0 || holds(group.Chats ?? [], here)
}

// holds is whether one of the conversations is the one at here.
export function holds(chats: RailChat[], here: string): boolean {
  return chats.some((chat) => chat.Where === here)
}
