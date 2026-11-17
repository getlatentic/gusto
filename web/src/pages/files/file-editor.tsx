import { useState, type FormEvent } from "react"

import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { withBase } from "@/lib/base"

// Opened is a text file as it was when the person opened it to change it: all of it, and its hash, which the save sends so the
// server refuses it when the file has changed since, and nobody's change is put over without them seeing it.
export interface Opened {
  text: string
  seen: string
}

// saveText sends what the person wrote, and gives why it was not saved when it was not.
async function saveText(save: string, form: HTMLFormElement): Promise<string> {
  const answer = await fetch(withBase(save), {
    method: "POST",
    credentials: "same-origin",
    headers: { Accept: "application/json" },
    body: new URLSearchParams(
      new FormData(form) as unknown as Record<string, string>
    ),
  })
  if (answer.ok) return ""
  const said = (await answer.json().catch(() => ({}))) as { error?: string }
  return said.error || "it was not saved"
}

// FileEditor is a text file opened to be changed. A save that is put in place is followed by the file as it is now.
export function FileEditor({
  name,
  save,
  opened,
  onSaved,
  onCancel,
}: {
  name: string
  save: string
  opened: Opened
  onSaved: () => void
  onCancel: () => void
}) {
  const [saving, setSaving] = useState(false)
  const [why, setWhy] = useState("")
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setSaving(true)
    setWhy("")
    try {
      const refused = await saveText(save, event.currentTarget)
      if (refused) setWhy(refused)
      else onSaved()
    } catch (err) {
      setWhy(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }
  return (
    <form
      className="grid gap-3"
      method="post"
      action={withBase(save)}
      onSubmit={submit}
      data-testid="file-edit"
    >
      <input type="hidden" name="seen" value={opened.seen} />
      <Textarea
        name="text"
        autoFocus
        spellCheck={false}
        aria-label={`The text of ${name}`}
        defaultValue={opened.text}
        data-testid="file-text"
        className="[field-sizing:fixed] min-h-[60vh] resize-y font-mono text-sm leading-relaxed [tab-size:4] md:text-sm"
      />
      {why && (
        <p className="m-0 text-sm text-bad" role="alert">
          It was not saved: {why}
        </p>
      )}
      <div className="flex items-center gap-2">
        <Button type="submit" disabled={saving} data-testid="file-save">
          Save
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  )
}
