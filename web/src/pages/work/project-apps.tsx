import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { appIsRunning } from "@/domain/work"
import { Disclosure } from "@/pages/work/disclosure"
import { Field } from "@/pages/work/field"
import { FormFailed } from "@/pages/work/form-failed"
import { SectionCard } from "@/pages/work/section-card"
import type { ProjectApp, ProjectData } from "@/pages/work/types"
import { useWorkForm } from "@/pages/work/use-work-form"
import { withBase } from "@/lib/base"

// ProjectApps is the web apps the agent built that are shared, and, where this server runs them, the way to share another.
export function ProjectApps({ page }: { page: ProjectData }) {
  const form = useWorkForm()
  const apps = `/instances/${encodeURIComponent(page.Instance.ID)}/apps`
  return (
    <SectionCard title="Web apps" id="apps" testId="apps">
      {page.Apps.length > 0 && (
        <ul className="m-0 list-none divide-y border-y p-0">
          {page.Apps.map((app) => (
            <li
              key={app.ID}
              className="flex items-start justify-between gap-3 py-3"
              data-testid="app"
            >
              <AppLine app={app} />
              <form
                method="post"
                action={withBase(
                  `${apps}/${encodeURIComponent(app.ID)}/remove`
                )}
                onSubmit={form.submit}
                className="m-0"
              >
                <Button type="submit" variant="outline" size="sm">
                  {appIsRunning(app.State) ? "Stop" : "Dismiss"}
                </Button>
              </form>
            </li>
          ))}
        </ul>
      )}
      <FormFailed failed={form.failed} />
      {page.Runs ? (
        <Disclosure
          open={page.Apps.length === 0}
          summary="Share an app this agent built"
        >
          <form
            method="post"
            action={withBase(apps)}
            onSubmit={form.submit}
            className="grid gap-4"
          >
            <Field label="The line that starts it">
              <Input name="command" placeholder="node server.js" required />
            </Field>
            <Field label="The port it listens on">
              <Input
                name="port"
                inputMode="numeric"
                defaultValue="8080"
                required
              />
            </Field>
            <div>
              <Button type="submit" disabled={form.sending}>
                Start it and share it
              </Button>
            </div>
          </form>
          <p className="m-0 mt-3 text-xs text-muted-foreground">
            It runs on your computer, in this agent&apos;s folder, with the port
            in <code>$PORT</code>. It has to listen on <code>0.0.0.0</code> and
            use links that are relative, such as <code>style.css</code> and not{" "}
            <code>/style.css</code>. Colleagues signed in to this organization
            open it here, and nobody else can. The page it draws runs with no
            access to your account.
          </p>
        </Disclosure>
      ) : (
        page.Apps.length === 0 && (
          <p className="m-0 text-sm text-muted-foreground">
            A page this agent builds can be opened from the conversation it was
            built in, with Preview.
          </p>
        )
      )}
    </SectionCard>
  )
}

function AppLine({ app }: { app: ProjectApp }) {
  return (
    <div className="min-w-0">
      <p className="m-0 flex flex-wrap items-center gap-x-2 text-sm">
        {app.State === "live" ? (
          <a
            className="font-medium"
            href={withBase(app.Link)}
            target="_blank"
            rel="noopener"
          >
            Open the app
          </a>
        ) : app.State === "starting" ? (
          <span className="text-muted-foreground">Starting…</span>
        ) : app.State === "failed" ? (
          <span className="font-medium text-bad">It did not start</span>
        ) : (
          <span className="text-muted-foreground">Stopped</span>
        )}
        {app.Folder ? (
          <span className="text-muted-foreground">
            the page in <code className="text-xs">{app.Folder}</code>
          </span>
        ) : (
          <span className="text-muted-foreground">
            <code className="text-xs">{app.Command}</code> on {app.Port}
          </span>
        )}
      </p>
      {app.Note && (
        <pre className="m-0 mt-1 text-xs whitespace-pre-wrap text-muted-foreground">
          {app.Note}
        </pre>
      )}
    </div>
  )
}
