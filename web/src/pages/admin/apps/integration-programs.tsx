import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import type { Integration, ShownServer } from "@/pages/admin/apps/types"

// Programs are the servers a version brought to run itself. They run only once an admin allows them, for that version only.
export function Programs({
  held,
  version,
  programs,
}: {
  held: Integration
  version: string
  programs: ShownServer[]
}) {
  const base = `/admin/integrations/${held.Name}/programs`
  return (
    <>
      <h3 className="m-0 mt-2 text-sm font-semibold">Programs</h3>
      <p className="m-0 max-w-prose text-muted-foreground">
        These servers are programs the plugin brought. They run only when you
        allow it, for this version only, each in a box of its own: no network
        unless this server was set up to give one, no access to the task or to
        any credential, and nothing it can write but a folder of the
        task&apos;s.
      </p>
      <ul className="m-0 grid list-none gap-1.5 p-0 break-words">
        {programs.map((server) => (
          <li key={server.Name}>
            <code className="font-mono">
              {[server.Command, ...(server.Args ?? [])].join(" ")}
            </code>{" "}
            <span className="text-muted-foreground">({server.Name})</span>
          </li>
        ))}
      </ul>
      {held.Programs === version ? (
        <>
          <p className="m-0">
            <strong>Allowed.</strong>
          </p>
          <form method="post" action={`${base}/refuse`} className="m-0">
            <Button variant="outline" size="sm" type="submit">
              Stop allowing them
            </Button>
          </form>
        </>
      ) : (
        <form
          className="grid max-w-prose gap-3"
          method="post"
          action={`${base}/allow`}
        >
          <input type="hidden" name="version" value={version} />
          <Label className="items-start font-normal">
            <Checkbox name="understood" value="yes" className="mt-0.5" />I have
            read what these programs are and I allow this version&apos;s to run.
          </Label>
          <div>
            <Button type="submit">Allow the programs</Button>
          </div>
        </form>
      )}
    </>
  )
}
