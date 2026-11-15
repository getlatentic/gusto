// How a person's pages say what a skill set holds.

const counted = (n: number, what: string) => `${n} ${what}${n === 1 ? "" : "s"}`

// holds says what a skill set holds: "2 skills · tools: memory, search".
export function holds(set: { Skills: number; Servers: string[] }): string {
  const skills = counted(set.Skills, "skill")
  return set.Servers.length > 0
    ? `${skills} · tools: ${set.Servers.join(", ")}`
    : skills
}

// serversLine is the MCP servers a skill set is given, as its page says them, with who changes them.
export function serversLine(servers: string[]): string {
  return `${servers.length > 0 ? servers.join(", ") : "None chosen."} An administrator changes these.`
}
