import { useRef } from "react"

import { Notice } from "@/components/page"
import { Badge } from "@/components/ui/badge"
import { Card } from "@/components/ui/card"
import { Switch } from "@/components/ui/switch"
import { HowThisWorks } from "@/components/disclosure"
import type { AppRow } from "@/pages/admin/apps/types"

// The apps an admin turns on for everyone. Each person then connects their own account to one, and their agents use it as them.
export function AppsList({
  apps,
  turnedOn,
}: {
  apps: AppRow[]
  turnedOn: string
}) {
  return (
    <>
      {turnedOn && (
        <Notice tone="plain">
          {turnedOn} is on. Each person connects their own account from
          Connections before their agents can use it.
        </Notice>
      )}
      <Card className="gap-0 py-0">
        <ul className="m-0 list-none divide-y p-0" aria-label="Apps">
          {apps.map((app) => (
            <AppLine key={app.ID} app={app} />
          ))}
        </ul>
      </Card>
      <p className="m-0 mt-3 text-sm text-muted-foreground">
        Turning an app on lets each person connect their own account. Agents use
        it only as that person, and only for what they ask.
      </p>
      <HowThisWorks title="How this works" className="mt-6">
        Each person signs in to an app that is on from Connections, with their
        own account there. Their agents then call it as them, through the
        gateway: the agent never holds the token, and sees what that person can
        see in the app, no more. In a room, an agent uses a person&apos;s apps
        only for what that person asks, and only the apps they let that agent
        use, app by app on its Apps tab or by their default on Connections.
        Turning an app off stops it from the next task. An app that needs setup
        is registered by whoever runs this server, with its client id in{" "}
        <code className="font-mono">GUSTO_CONNECTOR_CLIENTS</code>.
      </HowThisWorks>
    </>
  )
}

function AppLine({ app }: { app: AppRow }) {
  return (
    <li className="flex items-center gap-4 px-4 py-3">
      <div className="min-w-0 flex-1">
        <p className="m-0 text-sm font-medium">{app.Name}</p>
        <p className="m-0 text-sm text-muted-foreground">{app.What}</p>
        {app.NeedSignIn > 0 && (
          <p className="m-0 mt-0.5 text-xs text-warn">
            {app.NeedSignIn}{" "}
            {app.NeedSignIn === 1 ? "person needs" : "people need"} to sign in
            again
          </p>
        )}
      </div>
      {app.On || app.Usable ? (
        <AppSwitch app={app} />
      ) : (
        <Badge
          variant="outline"
          title="Whoever runs this server registers it with the app first"
        >
          needs setup
        </Badge>
      )}
    </li>
  )
}

// AppSwitch is the app's on and off for everyone. Flipping it sends its form, so the server turns the app and draws the page again.
function AppSwitch({ app }: { app: AppRow }) {
  const form = useRef<HTMLFormElement>(null)
  return (
    <form
      ref={form}
      method="post"
      action={`/admin/connectors/${app.ID}/${app.On ? "off" : "on"}`}
      className="m-0 flex flex-none"
    >
      <Switch
        checked={app.On}
        onCheckedChange={() => form.current?.requestSubmit()}
        aria-label={`${app.Name} for everyone`}
        title={`Turn ${app.Name} ${app.On ? "off" : "on"}`}
      />
    </form>
  )
}
