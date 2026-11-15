import type { SkillSetUse } from "@/components/skill-set/types"

// NoServers says why a skill set has no MCP servers to be given.
export function NoServers() {
  return (
    <p className="m-0 text-sm text-muted-foreground">
      This deployment offers no MCP servers. Its operator declares them, because
      each one runs inside the box or is an address the box may reach.
    </p>
  )
}

const counted = (n: number, what: string) => `${n} ${what}${n === 1 ? "" : "s"}`

// UseLine is what one plugin the set names gives it now, or why a task with the set would not start.
export function UseLine({ use }: { use: SkillSetUse }) {
  return (
    <p className="m-0 text-sm text-muted-foreground">
      {use.Name}:{" "}
      {use.On
        ? `${counted(use.Skills, "skill")}, ${counted(use.Servers, "server")}` +
          (use.Held ?? []).map((held) => ` · not used: ${held}`).join("")
        : "off, so a task with this setup does not start until an administrator turns it on"}
    </p>
  )
}
