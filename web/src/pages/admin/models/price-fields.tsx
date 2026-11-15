import { Input } from "@/components/ui/input"
import { Field } from "./form-dialog"
import type { Price } from "./types"

// perMillion is a price as the server wrote it: the number of dollars, as typed.
export const perMillion = (dollars: number) => `$${dollars}`

// PriceFields are a model's three prices, in dollars for a million tokens. Input and output are both given, or neither.
export function PriceFields({
  id,
  price,
  hints,
}: {
  id: string
  price?: Price | null
  hints: { input: string; output: string; cached: string }
}) {
  const cached = price?.cachedInputPerMillion
  return (
    <div className="grid grid-cols-3 gap-3 max-sm:grid-cols-1">
      <Field id={`${id}-input`} label="Input">
        <Input
          id={`${id}-input`}
          name="input"
          inputMode="decimal"
          className="font-numeric tabular-nums"
          placeholder={hints.input}
          defaultValue={price ? String(price.inputPerMillion) : undefined}
        />
      </Field>
      <Field id={`${id}-output`} label="Output">
        <Input
          id={`${id}-output`}
          name="output"
          inputMode="decimal"
          className="font-numeric tabular-nums"
          placeholder={hints.output}
          defaultValue={price ? String(price.outputPerMillion) : undefined}
        />
      </Field>
      <Field id={`${id}-cached`} label="Cached input">
        <Input
          id={`${id}-cached`}
          name="cached"
          inputMode="decimal"
          className="font-numeric tabular-nums"
          placeholder={hints.cached}
          defaultValue={cached !== undefined ? String(cached) : undefined}
        />
      </Field>
    </div>
  )
}
