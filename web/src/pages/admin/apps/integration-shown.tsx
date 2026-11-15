import type { ReactNode } from "react"

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Programs } from "@/pages/admin/apps/integration-programs"
import type {
  Integration,
  ShownServer,
  ShownVersion,
  VersionChanges,
} from "@/pages/admin/apps/types"

// ShownVersionCard is what one version holds: its skills, its MCP servers, the programs it brought, and what was left out.
export function ShownVersionCard({
  held,
  shown,
}: {
  held: Integration
  shown: ShownVersion
}) {
  const programs = shown.Servers.filter(
    (server) => server.Transport === "stdio"
  )
  return (
    <Card>
      <CardHeader>
        <CardTitle>{shown.Label || shown.ID}</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-3 text-sm">
        {shown.Description && <p className="m-0">{shown.Description}</p>}
        <Part title="Skills" count={shown.Skills.length}>
          {shown.Skills.map((skill) => (
            <li key={skill.Name}>
              <strong>{skill.Name}</strong>{" "}
              <span className="text-muted-foreground">{skill.Description}</span>
            </li>
          ))}
        </Part>
        <Part title="MCP servers" count={shown.Servers.length}>
          {shown.Servers.map((server) => (
            <ServerLine key={server.Name} server={server} />
          ))}
        </Part>
        {programs.length > 0 && (
          <Programs held={held} version={shown.ID} programs={programs} />
        )}
        {shown.Problems && shown.Problems.length > 0 && (
          <Part title="Left out or ignored">
            {shown.Problems.map((problem) => (
              <li key={problem.where + problem.message}>
                <code className="font-mono">{problem.where}</code>{" "}
                {problem.message}
                {problem.left && (
                  <span className="text-muted-foreground"> · left out</span>
                )}
              </li>
            ))}
          </Part>
        )}
      </CardContent>
    </Card>
  )
}

// Part is one titled list of a version's card, saying none when it holds nothing.
function Part({
  title,
  count,
  children,
}: {
  title: string
  count?: number
  children: ReactNode[]
}) {
  return (
    <>
      <h3 className="m-0 mt-2 text-sm font-semibold">
        {title}
        {count !== undefined && (
          <span className="font-normal text-muted-foreground"> {count}</span>
        )}
      </h3>
      <ul className="m-0 grid list-none gap-1.5 p-0 break-words">
        {children.length > 0 ? (
          children
        ) : (
          <li className="text-muted-foreground">none</li>
        )}
      </ul>
    </>
  )
}

const transportNotes: Record<string, string> = {
  stdio: "waits for the gateway",
  sse: "older transport, not used yet",
}

function ServerLine({ server }: { server: ShownServer }) {
  const note = transportNotes[server.Transport]
  return (
    <li>
      <strong>{server.Name}</strong>{" "}
      <span className="text-muted-foreground">
        {server.Transport}
        {server.URL && ` ${server.URL}`}
        {server.Command && ` runs ${server.Command}`}
      </span>
      {note && <span className="text-muted-foreground"> · {note}</span>}
    </li>
  )
}

const changeLines: { key: keyof VersionChanges; label: ReactNode }[] = [
  { key: "SkillsAdded", label: "Skills added" },
  { key: "SkillsRemoved", label: "Skills removed" },
  { key: "SkillsChanged", label: "Skills changed" },
  { key: "ServersAdded", label: "Servers added" },
  { key: "ServersRemoved", label: "Servers removed" },
  {
    key: "ServersChanged",
    label: (
      <>
        <strong>Servers changed</strong> (address, program or headers)
      </>
    ),
  },
  { key: "Manifest", label: "Described differently" },
]

// ChangesCard is what turning the version shown on would change, against the version that is on.
export function ChangesCard({ changes }: { changes: VersionChanges }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          What turning this on changes{" "}
          <span className="text-sm font-normal text-muted-foreground">
            against the version that is on
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent className="text-sm">
        {changes.None ? (
          <p className="m-0 text-muted-foreground">
            Nothing differs in skills, servers or manifest.
          </p>
        ) : (
          <ul className="m-0 grid list-none gap-1.5 p-0 break-words">
            {changeLines.map(({ key, label }) => {
              const names = changes[key]
              if (!Array.isArray(names) || names.length === 0) return null
              return (
                <li key={key}>
                  {label}: {names.join(", ")}
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
