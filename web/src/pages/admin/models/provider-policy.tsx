import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { FormDialog } from "./form-dialog"
import type { KeySource, ModelsView } from "./types"

// ProviderPolicy says how far a provider may reach. An installation that is the admin's own lets them change it; otherwise
// its operator fixes it.
export function ProviderPolicy({
  view,
  sources,
}: {
  view: ModelsView
  sources: KeySource[]
}) {
  if (!view.Editable) {
    return (
      <p className="m-0 mt-3 max-w-prose text-xs text-muted-foreground">
        This deployment&apos;s operator fixes where providers may be and how a
        key may be named: a provider must be at a public https address, and a
        key is typed here.{view.Private && " Private addresses are allowed."}
      </p>
    )
  }
  const named = sources
    .filter((source) => source.On)
    .map((source) => ` or named as ${source.Words}`)
    .join("")
  return (
    <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-2 text-xs text-muted-foreground">
      <span>
        A provider {view.Private ? "may" : "may not"} be at a private address,
        and its key is typed here{named}.
      </span>
      <PolicyDialog view={view} sources={sources} />
    </div>
  )
}

function PolicyDialog({
  view,
  sources,
}: {
  view: ModelsView
  sources: KeySource[]
}) {
  return (
    <FormDialog
      trigger={<Button variant="outline" size="sm" />}
      label="Change"
      title="What providers may be"
      words="This installation is yours, so you decide how far a provider may reach."
      action="/admin/models/policy"
      submit="Save"
    >
      <p className="m-0 text-sm text-muted-foreground">
        Each of these lets whoever adds a provider point this machine, or the
        keys it can read, at an address or a file of their choosing. Leave them
        off unless you run a model inside your own network or keep keys in a
        store this machine can read.
      </p>
      <Label className="font-normal">
        <Checkbox name="private" value="on" defaultChecked={view.Private} />A
        provider may be at a private address, this machine, or plain http
      </Label>
      <fieldset className="m-0 grid gap-2 border-0 p-0">
        <legend className="mb-2 text-sm font-medium">
          A key may be named as a reference to
        </legend>
        {sources.map((source) => (
          <Label key={source.Kind} className="font-normal">
            <Checkbox
              name="keys"
              value={source.Kind}
              defaultChecked={source.On}
            />
            {source.Words}
          </Label>
        ))}
      </fieldset>
    </FormDialog>
  )
}
