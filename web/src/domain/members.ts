// What the members pages decide from their data: which members the list shows, which of an agent's tabs is open, and the words
// built from what the server gave.

export type MemberTab = "all" | "people" | "agents"

export const memberTabs: { key: MemberTab; label: string }[] = [
  { key: "all", label: "All" },
  { key: "people", label: "People" },
  { key: "agents", label: "Agents" },
]

// memberTabOf is the tab a stored word names, and All for anything else.
export function memberTabOf(stored: string | null): MemberTab {
  return memberTabs.find((tab) => tab.key === stored)?.key ?? "all"
}

// shownIn is which members a tab lists.
export function shownIn(tab: MemberTab): { people: boolean; agents: boolean } {
  return { people: tab !== "agents", agents: tab !== "people" }
}

export type AgentTab = "profile" | "access" | "skills" | "apps"

// agentTabs are an agent's tabs: Apps only where the server offers apps.
export function agentTabs(
  hasApps: boolean
): { key: AgentTab; label: string }[] {
  const tabs: { key: AgentTab; label: string }[] = [
    { key: "profile", label: "Profile" },
    { key: "access", label: "Access" },
    { key: "skills", label: "Skills" },
  ]
  return hasApps ? [...tabs, { key: "apps", label: "Apps" }] : tabs
}

// agentTabOf is the tab the address asks for, when the agent has it, and the profile otherwise.
export function agentTabOf(asked: string | null, hasApps: boolean): AgentTab {
  return agentTabs(hasApps).find((tab) => tab.key === asked)?.key ?? "profile"
}

// KindNote is a line under an agent type in the new agent dialog, and whether it warns.
export interface KindNote {
  text: string
  warn: boolean
  // title is the whole of a line that may be cut short.
  title?: string
}

// kindNotes is what the new agent dialog says under an agent type: what it is and how it did here when it can run, and why not
// when it cannot.
export function kindNotes(kind: {
  Ready: boolean
  What: string
  Measured: string
  Failed: boolean
  Installable: boolean
  Why: string
}): KindNote[] {
  if (kind.Ready) {
    const notes: KindNote[] = []
    if (kind.What) notes.push({ text: kind.What, warn: false })
    if (kind.Measured) notes.push({ text: kind.Measured, warn: kind.Failed })
    return notes
  }
  if (kind.Installable)
    return [{ text: "Not on this computer yet", warn: false }]
  return [
    { text: `Not on this server: ${kind.Why}`, warn: false, title: kind.Why },
  ]
}

// emailWords is what a person's page says of their email address.
export function emailWords(person: {
  Who: string
  SharedSignIn: boolean
}): string {
  return person.SharedSignIn
    ? "Signs in with this server's password"
    : person.Who
}

// appChoices are what an agent may do with one of the person's apps; inherit says what the person's default is.
export function appChoices(
  byDefault: boolean
): { value: string; label: string }[] {
  return [
    { value: "inherit", label: `Inherit (${byDefault ? "on" : "off"})` },
    { value: "allow", label: "Allow" },
    { value: "deny", label: "Don’t allow" },
  ]
}

// skillGroups splits what an agent can be given into the ready-made roles and the workspace's skill sets.
export function skillGroups<T extends { Role: boolean }>(
  choices: T[]
): { roles: T[]; sets: T[] } {
  return {
    roles: choices.filter((choice) => choice.Role),
    sets: choices.filter((choice) => !choice.Role),
  }
}
