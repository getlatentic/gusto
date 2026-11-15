import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { ChoiceSelect } from "./choice-select"
import { Field, FormDialog } from "./form-dialog"
import { PostButton } from "./post-button"
import { Steps } from "./steps"
import type { DefaultChoice, ModelsData, RunTokens } from "./types"

// SettingsTab is what a new chat runs on, how sandboxes reach a model, and the earlier versions of the registry.
export function SettingsTab({ page }: { page: ModelsData }) {
  const defaults = page.Defaults ?? []
  return (
    <div className="grid gap-8">
      {(defaults.length > 0 || page.RunTokens) && (
        <Card className="gap-0 divide-y px-4 py-0">
          {defaults.length > 0 && (
            <ChatDefault defaults={defaults} now={page.DefaultNow ?? ""} />
          )}
          {page.RunTokens && <SandboxTokens tokens={page.RunTokens} />}
        </Card>
      )}
      {page.Steps && page.Steps.length > 0 && (
        <Steps steps={page.Steps} pages={page.Pages} />
      )}
    </div>
  )
}

function ChatDefault({
  defaults,
  now,
}: {
  defaults: DefaultChoice[]
  now: string
}) {
  const choices = [
    { value: "", label: "The first that can run" },
    ...defaults.map((each) => ({ value: each.Value, label: each.Label })),
  ]
  const chosen = defaults.find((each) => each.Chosen)?.Value ?? ""
  return (
    <section className="flex flex-wrap items-center justify-between gap-3 py-4">
      <div className="grid gap-0.5">
        <h2 className="m-0 text-sm font-semibold">A new chat starts on</h2>
        <p className="m-0 text-sm text-muted-foreground">
          {now}. A person&apos;s own last choice comes first.
        </p>
      </div>
      <FormDialog
        trigger={<Button variant="outline" />}
        label="Change"
        title="A new chat starts on"
        words="What a new chat runs on until a person chooses their own. A person's last choice comes first."
        action="/admin/chat-default"
        submit="Save"
      >
        <Field id="chat-default" label="Agent and model">
          <ChoiceSelect
            id="chat-default"
            name="default"
            choices={choices}
            defaultValue={chosen}
          />
        </Field>
      </FormDialog>
    </section>
  )
}

function SandboxTokens({ tokens }: { tokens: RunTokens }) {
  return (
    <section className="flex flex-wrap items-center justify-between gap-3 py-4">
      <div className="grid max-w-prose gap-0.5">
        <h2 className="m-0 text-sm font-semibold">
          Advanced: how sandboxes reach a model
        </h2>
        {tokens.Err && (
          <p className="m-0 text-sm text-bad">
            The setting could not be read, so runs are bound. {tokens.Err}
          </p>
        )}
        <p className="m-0 text-sm text-muted-foreground">
          {tokens.Bound
            ? "Each run's model access is bound to its sandbox. What an agent could copy out is no use anywhere else."
            : "Each run gets a plain token for its model, which works from anywhere until the run ends."}{" "}
          A change takes effect on the next run.
        </p>
      </div>
      <PostButton
        action="/admin/run-tokens"
        fields={{ bound: tokens.Bound ? "no" : "yes" }}
        variant={tokens.Bound ? "outline" : "default"}
      >
        {tokens.Bound ? "Use plain tokens" : "Bind to the sandbox"}
      </PostButton>
    </section>
  )
}
