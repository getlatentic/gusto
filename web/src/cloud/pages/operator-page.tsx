import type { ReactNode } from "react"

import { Field } from "@/cloud/components/forms"
import type { OperatorView } from "@/cloud/types"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// OperatorPage is where the operator of a hosted gusto manages it: whether new demos are taken, how many may start, which are
// running, the hosts that are up, and who asked for one.
export function OperatorPage({ page }: { page: OperatorView }) {
  return (
    <div className="min-h-dvh bg-background">
      <header className="border-b bg-card px-4 py-3">
        <p className="m-0 mx-auto flex max-w-5xl items-center gap-2 font-heading text-base font-semibold">
          <img className="w-[1.375rem]" src="/static/mark.svg" alt="" /> gusto
          operator
        </p>
      </header>
      <main className="mx-auto grid max-w-5xl gap-4 px-4 pt-8 pb-20">
        {page.Notice && (
          <p
            className="m-0 rounded-md bg-secondary px-4 py-3 text-sm"
            role="status"
            data-testid="notice"
          >
            {page.Notice}
          </p>
        )}
        <h1 className="m-0 text-2xl font-semibold">Demos</h1>
        <Pause paused={page.Paused} />
        <Limits page={page} />
        <Running rows={page.Running ?? []} />
        <Hosts hosts={page.Hosts} />
        <Leads leads={page.Leads ?? []} />
        <Contacts contacts={page.Contacts ?? []} />
      </main>
    </div>
  )
}

function Panel({ children }: { children: ReactNode }) {
  return (
    <Card className="py-5">
      <CardContent className="grid gap-4 px-5">{children}</CardContent>
    </Card>
  )
}

function Heading({ children }: { children: ReactNode }) {
  return <h2 className="m-0 mt-4 text-lg font-semibold">{children}</h2>
}

function Quiet({ children }: { children: ReactNode }) {
  return <p className="m-0 text-sm text-muted-foreground">{children}</p>
}

function Pause({ paused }: { paused: boolean }) {
  return (
    <Panel>
      <form className="grid gap-4" method="post" action="/demos/pause">
        <p className="m-0" data-testid="demos-state">
          {paused ? (
            <>
              <strong>New demos are paused.</strong> Visitors are told to try
              again later. Demos that are running go on.
            </>
          ) : (
            "New demos are being accepted."
          )}
        </p>
        <input type="hidden" name="paused" value={paused ? "off" : "on"} />
        <div>
          <Button type="submit" data-testid="pause">
            {paused ? "Accept new demos" : "Pause new demos"}
          </Button>
        </div>
      </form>
    </Panel>
  )
}

function Limits({ page }: { page: OperatorView }) {
  const limits = page.Limits
  return (
    <>
      <Heading>Limits</Heading>
      <Panel>
        <form
          className="grid max-w-prose gap-4"
          method="post"
          action="/demos/limits"
        >
          <Quiet>
            How many demos may start. A limit of 0 is no limit. A day is the
            last twenty-four hours, and a person or a company is counted from
            the leads, so it holds after their demos are deleted.{" "}
            {page.LimitsSet
              ? "These are the limits you set."
              : "These are the limits gusto cloud started with."}
          </Quiet>
          <Field
            label="Most in a day, in all"
            type="number"
            name="perDay"
            min={0}
            max={1000}
            defaultValue={limits.perDay}
            required
          />
          <Field
            label="Most one address may start in a day"
            type="number"
            name="perPerson"
            min={0}
            max={1000}
            defaultValue={limits.perPerson}
            required
          />
          <Field
            label="Most one company may start in a day"
            type="number"
            name="perCompany"
            min={0}
            max={1000}
            defaultValue={limits.perCompany}
            required
          />
          <Field
            label="Most running at once"
            type="number"
            name="maxOrgs"
            min={1}
            max={50}
            defaultValue={limits.maxOrgs}
            required
          />
          <div className="flex flex-wrap gap-3">
            <Button type="submit" data-testid="save-limits">
              Save the limits
            </Button>
            {page.LimitsSet && (
              <Button
                variant="outline"
                type="submit"
                name="reset"
                value="1"
                formNoValidate
              >
                Go back to the starting limits
              </Button>
            )}
          </div>
        </form>
      </Panel>
    </>
  )
}

function Rows({
  heads,
  rows,
  testId,
}: {
  heads: string[]
  rows: { key: string; cells: ReactNode[]; title?: string }[]
  testId: string
}) {
  return (
    <Card className="gap-0 overflow-hidden py-0">
      <Table data-testid={testId}>
        <TableHeader>
          <TableRow>
            {heads.map((head) => (
              <TableHead key={head}>{head}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => (
            <TableRow key={row.key}>
              {row.cells.map((cell, index) => (
                <TableCell key={index}>{cell}</TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  )
}

function Running({ rows }: { rows: NonNullable<OperatorView["Running"]> }) {
  return (
    <>
      <Heading>Running now</Heading>
      {rows.length ? (
        <Rows
          testId="running"
          heads={["Who", "Company", "State", "Time left"]}
          rows={rows.map((row, index) => ({
            key: `${row.Name}-${index}`,
            cells: [row.Name, row.Company, row.State, row.Left],
          }))}
        />
      ) : (
        <Quiet>No demo is running.</Quiet>
      )}
    </>
  )
}

function Hosts({ hosts }: { hosts: OperatorView["Hosts"] }) {
  const rows = hosts.Rows ?? []
  return (
    <>
      <Heading>Hosts</Heading>
      {hosts.Error ? (
        <p className="m-0 rounded-md bg-bad-wash px-4 py-3 text-sm text-bad">
          The hosts could not be listed: {hosts.Error}
        </p>
      ) : !hosts.Known ? (
        <Quiet>Hosts are not started on demand here.</Quiet>
      ) : rows.length ? (
        <Rows
          testId="hosts"
          heads={["Host", "Up since"]}
          rows={rows.map((row) => ({ key: row.ID, cells: [row.ID, row.Up] }))}
        />
      ) : (
        <Quiet>
          No host is running for organizations. One starts when a demo is asked
          for.
        </Quiet>
      )}
    </>
  )
}

function Leads({ leads }: { leads: NonNullable<OperatorView["Leads"]> }) {
  return (
    <>
      <Heading>Leads</Heading>
      <Quiet>
        <a
          className="text-foreground underline underline-offset-2"
          href="/leads.csv"
        >
          Download as a spreadsheet
        </a>{" "}
        · {leads.length} so far
      </Quiet>
      {leads.length ? (
        <Rows
          testId="leads"
          heads={["Asked", "Name", "Email", "Company", "Outcome"]}
          rows={leads.map((lead, index) => ({
            key: `${lead.Email}-${index}`,
            cells: [
              <span className="whitespace-nowrap" title={lead.AskedAt}>
                {lead.Asked}
              </span>,
              lead.Name,
              lead.Email,
              lead.Company,
              lead.Outcome,
            ],
          }))}
        />
      ) : (
        <Quiet>Nobody has asked for a demo yet.</Quiet>
      )}
    </>
  )
}

// Contacts are the notes visitors sent from the landing page, newest first, which are also mailed to the team.
function Contacts({
  contacts,
}: {
  contacts: NonNullable<OperatorView["Contacts"]>
}) {
  return (
    <>
      <Heading>Notes from visitors</Heading>
      <Quiet>
        <a
          className="text-foreground underline underline-offset-2"
          href="/contacts.csv"
        >
          Download as a spreadsheet
        </a>{" "}
        · the newest {contacts.length}
      </Quiet>
      {contacts.length ? (
        <Rows
          testId="contacts"
          heads={["Sent", "Email", "Company", "Note"]}
          rows={contacts.map((contact, index) => ({
            key: `${contact.Email}-${index}`,
            cells: [
              <span className="whitespace-nowrap" title={contact.SentAt}>
                {contact.Sent}
              </span>,
              contact.Email,
              contact.Company,
              <span className="block max-w-xl whitespace-pre-wrap">
                {contact.Note}
              </span>,
            ],
          }))}
        />
      ) : (
        <Quiet>No visitor has sent a note yet.</Quiet>
      )}
    </>
  )
}
