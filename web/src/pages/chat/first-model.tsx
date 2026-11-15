import { useRef, useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { postForm } from "@/lib/http"
import type { ChatData } from "@/pages/chat/types"

const field = "grid gap-2"
const another = "another"

interface Listed {
  models?: string[] | null
  error?: string
}

// listedWords is what the person is told after asking the provider which models the key can use.
function listedWords(listed: Listed | undefined, failed: boolean): string {
  if (failed) return "The provider could not be asked."
  if (!listed) return ""
  if (listed.error) return listed.error
  return `${listed.models?.length ?? 0} models. Start typing one, or pick from the list.`
}

// FirstModel adds the model a person uses, before anything else can run: a provider the app knows fills in where it answers and
// how; another provider asks for them. List my models asks the provider which models the key can use.
export function FirstModel({ page }: { page: ChatData }) {
  const [provider, setProvider] = useState(page.Presets[0]?.Address ?? another)
  const known = page.Presets.find((each) => each.Address === provider)
  const form = useRef<HTMLFormElement>(null)
  const models = useRef<HTMLInputElement>(null)
  // listing is the provider's answer to which models the key can use: asked, answered, or not reached.
  const [listing, setListing] = useState<{
    asking: boolean
    listed?: Listed
    failed?: boolean
  }>({ asking: false })
  const list = () => {
    const fields = new FormData(form.current!)
    const asked = new FormData()
    for (const name of ["address", "auth", "key"])
      asked.set(name, String(fields.get(name) ?? ""))
    setListing({ asking: true })
    postForm<Listed>("/setup/models", asked).then(
      (listed) => {
        setListing({ asking: false, listed })
        if (!listed.error) models.current?.focus()
      },
      () => setListing({ asking: false, failed: true })
    )
  }
  const said = listing.asking
    ? "Asking the provider…"
    : listedWords(listing.listed, Boolean(listing.failed))
  return (
    <>
      <form
        ref={form}
        method="post"
        action="/setup/model"
        className="grid w-full max-w-136 gap-6"
      >
        <div className="grid gap-4">
          <p className="m-0">
            Every agent can use it. Its key stays on this computer with gusto,
            and no agent is given it.
          </p>
          {(page.FoundKeys ?? []).map((key) => (
            <p key={key} className="m-0 text-sm text-muted-foreground">
              Your shell has {key}. To use it, run{" "}
              <code className="rounded-xs bg-muted px-1 font-mono text-xs wrap-anywhere">
                gusto local use {key}
              </code>{" "}
              in a terminal.
            </p>
          ))}
        </div>
        <fieldset className="m-0 grid gap-4 border-0 p-0">
          <div className={field}>
            <Label htmlFor="provider">Provider</Label>
            <select
              id="provider"
              className="h-9 rounded-md border border-input bg-transparent px-3 text-sm shadow-xs"
              value={provider}
              onChange={(event) => setProvider(event.target.value)}
            >
              {page.Presets.map((each) => (
                <option key={each.Address} value={each.Address}>
                  {each.Title}
                </option>
              ))}
              <option value={another}>Another provider</option>
            </select>
          </div>
          {known ? (
            <>
              <input type="hidden" name="address" value={known.Address} />
              <input
                type="hidden"
                name="auth"
                value={known.Auth === "anthropic" ? "anthropic" : ""}
              />
            </>
          ) : (
            <>
              <div className={field}>
                <Label htmlFor="address">Address</Label>
                <Input
                  id="address"
                  name="address"
                  required
                  placeholder="https://api.example.com"
                  autoFocus
                />
              </div>
              <div className={field}>
                <Label htmlFor="auth">Answers like</Label>
                <select
                  id="auth"
                  name="auth"
                  className="h-9 rounded-md border border-input bg-transparent px-3 text-sm shadow-xs"
                >
                  <option value="">OpenAI&apos;s API</option>
                  <option value="anthropic">Anthropic&apos;s API</option>
                </select>
              </div>
            </>
          )}
          <div className={field}>
            <Label htmlFor="key">Key</Label>
            <Input id="key" name="key" type="password" autoComplete="off" />
          </div>
          <div className={field}>
            <Label htmlFor="models">Models</Label>
            <div className="flex gap-2">
              <Input
                ref={models}
                id="models"
                name="models"
                required
                placeholder="One or more, separated by commas"
                list="found-models"
                className="min-w-0 flex-1"
              />
              <Button type="button" variant="outline" onClick={list}>
                List my models
              </Button>
            </div>
            <p className="m-0 text-xs text-subtle empty:hidden" role="status">
              {said}
            </p>
            <datalist id="found-models">
              {(listing.listed?.models ?? []).map((name) => (
                <option key={name} value={name} />
              ))}
            </datalist>
          </div>
        </fieldset>
        <div>
          <Button type="submit">Add</Button>
        </div>
        {page.Missing.length > 0 && <Missing page={page} />}
      </form>
      {page.Missing.filter((agent) => agent.Installable).map((agent) => (
        <form
          key={agent.Name}
          id={`install-${agent.Name}`}
          method="post"
          action={`/agents/${agent.Name}/install`}
          hidden
        >
          <input type="hidden" name="back" value="/chat" />
        </form>
      ))}
    </>
  )
}

// Missing are the agents not on this computer yet: gusto installs some, and says the command for the others.
function Missing({ page }: { page: ChatData }) {
  return (
    <section
      aria-labelledby="missing-agents"
      className="grid gap-2 border-t pt-4"
    >
      <h2
        id="missing-agents"
        className="m-0 text-sm font-medium text-muted-foreground"
      >
        Not on this computer yet
      </h2>
      <dl className="m-0 grid grid-cols-[max-content_minmax(0,1fr)] items-center gap-x-4 gap-y-2 text-sm">
        {page.Missing.map((agent) => (
          <div key={agent.Name} className="contents">
            <dt>{agent.Title}</dt>
            <dd className="m-0">
              {agent.Installable ? (
                <Button
                  type="submit"
                  form={`install-${agent.Name}`}
                  variant="outline"
                  size="sm"
                >
                  Install
                </Button>
              ) : (
                <code className="rounded-xs bg-muted px-1 font-mono text-xs wrap-anywhere">
                  {agent.Why}
                </code>
              )}
            </dd>
          </div>
        ))}
      </dl>
      {page.HandInstall && (
        <p className="m-0 text-sm text-muted-foreground">
          After running a command above, run{" "}
          <code className="rounded-xs bg-muted px-1 font-mono text-xs">
            gusto open
          </code>{" "}
          in the same terminal, so gusto looks where that terminal does.
        </p>
      )}
    </section>
  )
}
