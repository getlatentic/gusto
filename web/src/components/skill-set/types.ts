// A skill set as both its pages show it, the console's and a person's, as internal/web builds it.

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
