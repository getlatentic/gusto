import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { withBase } from "@/lib/base"

// SavePreset keeps the current agent, model and role as a preset under a name.
export function SavePreset({
  harness,
  model,
  role,
  onCancel,
}: {
  harness: string
  model: string
  role: string
  onCancel: () => void
}) {
  return (
    <Card className="mt-3 p-3">
      <form
        method="post"
        action={withBase("/presets")}
        className="flex items-center gap-2"
      >
        <Input
          name="name"
          placeholder="Name this preset"
          maxLength={60}
          aria-label="Name of the preset"
          required
          autoFocus
          className="min-w-0 flex-1"
        />
        <input type="hidden" name="harness" value={harness} />
        <input type="hidden" name="model" value={model} />
        <input type="hidden" name="role" value={role} />
        <Button type="submit">Save preset</Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </form>
    </Card>
  )
}
