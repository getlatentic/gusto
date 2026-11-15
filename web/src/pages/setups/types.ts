import type { SkillOfSet, SkillSetUse } from "@/components/skill-set/types"
import type { Role } from "@/domain/roles"

// The views of the pages a person starts an agent from, as internal/web builds them at /app/api. Field names are the Go fields'.

export interface SetupRow {
  Name: string
  Skills: number
  Servers: string[]
}

// SetupsData is the skill sets that are on. Keeps is whether this server keeps skill sets at all.
export interface SetupsData {
  Setups: SetupRow[]
  Keeps: boolean
  Refused: string
}

// An agent type a skill set may be started with: where to start it, or why it cannot take the set.
export interface SetupStarter {
  Name: string
  Href: string
  Unfit: string
}

// SetupData is one skill set that is on. Offers is whether this deployment offers MCP servers at all.
export interface SetupData {
  Set: { Name: string; Off: boolean; Skills: SkillOfSet[] }
  Servers: string[]
  Offers: boolean
  Uses: SkillSetUse[]
  StartWith: SetupStarter[]
  Refused: string
}

// A preset the person saved, with what it holds in a sentence.
export interface PresetRow {
  ID: string
  Name: string
  Line: string
}

export interface PresetsData {
  Roles: Role[]
  Fields: string[]
  Presets: PresetRow[]
  Refused: string
}

export interface TemplateSkill {
  Title: string
  Blurb: string
  // Instructions is the skill's body as the server draws it from Markdown, with nothing in it that runs.
  Instructions: string
}

export interface Template {
  Name: string
  Title: string
  Industry: string
  Summary: string
  Restricted: boolean
  Skills: TemplateSkill[]
  Starters: string[]
  Files: string[]
}

export interface AgentType {
  Name: string
  Title: string
}

// TemplateData is one template and the agents here that can run it, the one it starts on first. InBuilding is whether a model
// inside the deployment's own network is reachable, which restricted work needs.
export interface TemplateData {
  Template: Template
  Harnesses: AgentType[]
  InBuilding: boolean
  Refused: string
}

export interface BlankData {
  Engines: (AgentType & { What: string })[]
  Setups: string[]
  Refused: string
}

export interface Choice {
  value: string
  label: string
}

// SetUpData is what starting one agent type offers. Mark is the type's logo, when it has one; Chosen is the skill set the link
// named, and Then what the person wants once it has started.
export interface SetUpData {
  Harness: AgentType & { What: string; Mark: string }
  Models: Choice[]
  Setups: Choice[]
  Chosen: string
  Then: string
  Refused: string
}
