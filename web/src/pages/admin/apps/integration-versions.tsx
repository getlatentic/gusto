import { AppLink } from "@/components/app-link"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { when } from "@/lib/format"
import { OnBadge } from "@/pages/admin/apps/on-badge"
import { Section } from "@/components/section"
import type {
  Integration,
  IntegrationEvent,
  Source,
  VersionRow,
} from "@/pages/admin/apps/types"

// OnFor chooses who the plugin is on for, by the addresses people sign in with.
export function OnFor({ held }: { held: Integration }) {
  const addresses = (held.For ?? []).map((each) => each + "\n").join("")
  return (
    <Card>
      <CardHeader>
        <CardTitle>Who it is on for</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-3">
        <p className="m-0 max-w-prose text-sm text-muted-foreground">
          Addresses people sign in with, one to a line:{" "}
          <code className="font-mono">ada@example.com</code>, or{" "}
          <code className="font-mono">*@example.com</code> for everybody at a
          company. Leave it empty for everybody. A team is a label a person
          chooses, and does not decide who may have what.
        </p>
        <form
          className="grid max-w-prose gap-3"
          method="post"
          action={`/admin/integrations/${held.Name}/for`}
        >
          <Textarea
            name="for"
            rows={3}
            aria-label="Addresses"
            defaultValue={addresses}
          />
          <div>
            <Button type="submit">Save</Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}

// sourceOf is where a version came from, and the commit it is pinned to.
export function sourceOf(source: Source): string {
  let said = source.kind
  if (source.address) said += ` ${source.address}`
  if (source.commit) said += ` @ ${source.commit}`
  return said
}

// Versions is every version imported, the one that is on marked, each other with the way to turn it on.
export function Versions({ held }: { held: Integration }) {
  const versions = held.Versions ?? []
  return (
    <section className="grid gap-2">
      <h2 className="m-0 text-sm font-semibold">
        Versions{" "}
        <span className="font-normal text-muted-foreground">
          {versions.length}
        </span>
      </h2>
      <Card className="@container gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Version</TableHead>
              <TableHead className="@max-2xl:hidden">From</TableHead>
              <TableHead className="@max-lg:hidden">Imported</TableHead>
              <TableHead>
                <span className="sr-only">Turn on</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {versions.map((version) => (
              <VersionLine key={version.ID} held={held} version={version} />
            ))}
          </TableBody>
        </Table>
      </Card>
    </section>
  )
}

function VersionLine({
  held,
  version,
}: {
  held: Integration
  version: VersionRow
}) {
  const base = `/admin/integrations/${held.Name}`
  return (
    <TableRow>
      <TableCell className="w-full max-w-0">
        <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <AppLink
            href={`${base}?version=${version.ID}`}
            className="font-medium text-foreground no-underline hover:underline"
          >
            {version.Label || version.ID}
          </AppLink>
          <span className="font-mono text-xs text-muted-foreground">
            {version.ID}
          </span>
          {version.ID === held.On && <OnBadge on />}
        </span>
      </TableCell>
      <TableCell className="max-w-80 truncate text-muted-foreground @max-2xl:hidden">
        {sourceOf(version.Source)}
      </TableCell>
      <TableCell className="whitespace-nowrap text-muted-foreground @max-lg:hidden">
        {when(version.Imported)} by {version.By}
      </TableCell>
      <TableCell className="text-right">
        {version.ID !== held.On && (
          <form method="post" action={`${base}/enable`} className="m-0">
            <input type="hidden" name="version" value={version.ID} />
            <Button variant="outline" size="sm" type="submit">
              Turn on
            </Button>
          </form>
        )}
      </TableCell>
    </TableRow>
  )
}

// IntegrationHistory is everything done to the plugin, when and by whom.
export function IntegrationHistory({
  history,
}: {
  history: IntegrationEvent[]
}) {
  return (
    <Section title="History">
      <ul className="m-0 grid list-none gap-1.5 p-0 text-sm text-muted-foreground">
        {history.map((event, i) => (
          <li key={i}>
            {when(event.at)} · {event.by} {event.action}
            {event.version && ` ${event.version}`}
          </li>
        ))}
      </ul>
    </Section>
  )
}
