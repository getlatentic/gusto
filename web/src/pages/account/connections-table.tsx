import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  actionsOf,
  connectPath,
  disconnectPath,
  forAgentsPath,
  standingOf,
  type Connection,
} from "@/domain/connections"
import { ago } from "@/lib/format"
import { FormSelect } from "@/components/form-select"
import type { AccountForm } from "@/pages/account/use-account-form"

const forAgentsChoices = [
  {
    choices: [
      { value: "on", label: "On" },
      { value: "off", label: "Off" },
    ],
  },
]

// ConnectionsTable is each app a person may sign in to, or still holds a sign-in to. On a narrow page what the agents reach goes
// under the app's name, and the cells draw closer.
export function ConnectionsTable({
  apps,
  form,
}: {
  apps: Connection[]
  form: AccountForm
}) {
  return (
    <Table className="@max-lg:[&_td]:px-2 @max-lg:[&_th]:px-2">
      <TableHeader>
        <TableRow>
          <TableHead>App</TableHead>
          <TableHead className="@max-2xl:hidden">
            What your agents reach
          </TableHead>
          <TableHead>Connected</TableHead>
          <TableHead className="whitespace-normal">For my agents</TableHead>
          <TableHead>
            <span className="sr-only">Sign-in</span>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {apps.map((app) => (
          <AppRow key={app.ID} app={app} form={form} />
        ))}
      </TableBody>
    </Table>
  )
}

function AppRow({ app, form }: { app: Connection; form: AccountForm }) {
  const actions = actionsOf(app)
  return (
    <TableRow data-testid="connection" data-app={app.ID}>
      <TableCell className="font-medium whitespace-normal">
        {app.Name}
        <span className="block text-xs font-normal text-muted-foreground @2xl:hidden">
          {app.What}
        </span>
      </TableCell>
      <TableCell className="whitespace-normal text-muted-foreground @max-2xl:hidden">
        {app.What}
      </TableCell>
      <TableCell>
        <Standing app={app} />
      </TableCell>
      <TableCell>
        {actions.forAgents ? (
          <form
            method="post"
            action={forAgentsPath(app.ID)}
            onSubmit={form.submit}
            className="m-0"
          >
            <FormSelect
              key={String(app.ForAgents)}
              name="on"
              label={`Whether your agents may use ${app.Name}, unless you choose otherwise for one`}
              groups={forAgentsChoices}
              value={app.ForAgents ? "on" : "off"}
              sendOnChoose
              className="h-8 w-20"
              testId="for-agents"
            />
          </form>
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </TableCell>
      <TableCell>
        <div className="flex flex-wrap justify-end gap-2">
          {actions.connect && (
            <a
              href={connectPath(app.ID)}
              className={buttonVariants({ variant: "outline", size: "sm" })}
              data-testid="connect"
            >
              {actions.connect}
            </a>
          )}
          {actions.disconnect && (
            <form
              method="post"
              action={disconnectPath(app.ID)}
              onSubmit={form.submit}
              className="m-0"
            >
              <Button
                type="submit"
                variant="outline"
                size="sm"
                disabled={form.sending}
                data-testid="disconnect"
              >
                Disconnect
              </Button>
            </form>
          )}
        </div>
      </TableCell>
    </TableRow>
  )
}

// Standing says whether the person is signed in to the app, since when, or why their agents cannot use it.
function Standing({ app }: { app: Connection }) {
  switch (standingOf(app)) {
    case "turned off":
      return (
        <Badge
          className="bg-warn-wash text-warn"
          title="An admin turned this app off, so your agents are not given it. Your sign-in is kept until you disconnect."
        >
          turned off
        </Badge>
      )
    case "connected":
      return (
        <Badge className="bg-ok-wash text-ok" data-testid="connected-since">
          {ago(app.Since)}
        </Badge>
      )
    case "sign in again":
      return (
        <Badge
          className="bg-warn-wash text-warn"
          title="The app no longer takes your sign-in, so your agents cannot use it"
        >
          sign in again
        </Badge>
      )
    default:
      return <span className="text-muted-foreground">not yet</span>
  }
}
