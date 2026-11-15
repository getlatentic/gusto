// The views of Apps & skills the server answers at /app/api, as internal/web builds them. Field names are the Go fields', and a
// type the server keeps as it is (an event, a source, a problem) keeps its own JSON names.

export interface AppsTab {
  Key: string
  Label: string
  Href: string
  On: boolean
}

// An app of the catalog: on for everyone or not, and whether whoever runs this server has set it up so it can be.
export interface AppRow {
  ID: string
  Name: string
  What: string
  On: boolean
  Usable: boolean
  NeedSignIn: number
}

// A skill set written here or a plugin imported, as one line of the skills list.
export interface SkillRow {
  Name: string
  Href: string
  Kind: string
  Detail: string
  On: boolean
}

// An imported plugin a skill set may be given, and whether the one shown is.
export interface ImportedChoice {
  Name: string
  On: boolean
  Chosen: boolean
}

export interface AppsData {
  Tabs: AppsTab[]
  Tab: "apps" | "skills"
  Refused: string
  Apps?: AppRow[]
  TurnedOn?: string
  Skills?: SkillRow[]
  Writes?: boolean
  Imports?: boolean
  Servers?: string[]
  Imported?: ImportedChoice[]
}

export interface Source {
  kind: string
  address?: string
  ref?: string
  commit?: string
}

export interface VersionRow {
  ID: string
  Label: string
  By: string
  Source: Source
  Imported: number
}

export interface IntegrationEvent {
  at: number
  by: string
  action: string
  version?: string
  was?: string
}

export interface Integration {
  Name: string
  On: string
  Before: string
  Programs: string
  For: string[] | null
  Versions: VersionRow[] | null
  History: IntegrationEvent[] | null
}

export interface ShownServer {
  Name: string
  Transport: string
  URL: string
  Command: string
  Args: string[] | null
}

export interface Problem {
  where: string
  message: string
  left?: boolean
}

// The version the page opens: what it holds, and what was left out of it.
export interface ShownVersion {
  ID: string
  Label: string
  Description: string
  Skills: { Name: string; Description: string }[]
  Servers: ShownServer[]
  Problems: Problem[] | null
}

// What turning the version shown on would change against the version that is on.
export interface VersionChanges {
  SkillsAdded: string[] | null
  SkillsRemoved: string[] | null
  SkillsChanged: string[] | null
  ServersAdded: string[] | null
  ServersRemoved: string[] | null
  ServersChanged: string[] | null
  Manifest: string[] | null
  None: boolean
}

export interface IntegrationData {
  Integration: Integration
  Shown?: ShownVersion
  Changes?: VersionChanges
  Against?: string
  Refused: string
}

export interface SkillOfSet {
  Name: string
  Description: string
  // Instructions is the skill's body as the server draws it from Markdown, with nothing in it that runs.
  Instructions: string
}

// What one integration a skill set names gives it now.
export interface SkillSetUse {
  Name: string
  Skills: number
  Servers: number
  Held: string[] | null
  On: boolean
}

export interface ServerChoice {
  Name: string
  Where: string
  Chosen: boolean
}

// A kept version of a skill. At names it, in nanoseconds, as a string.
export interface EarlierSkill {
  Skill: string
  At: string
  Fingerprint: string
  Present: boolean
}

export interface SkillSetData {
  Set: { Name: string; Off: boolean; Skills: SkillOfSet[] }
  Uses: SkillSetUse[]
  Servers: ServerChoice[]
  Imported: ImportedChoice[]
  Earlier: EarlierSkill[]
  Refused: string
}
