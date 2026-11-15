import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { AddressKind } from "./address-kind"
import { Field, FormDialog } from "@/components/form-dialog"
import { PriceFields } from "./price-fields"
import type { Provider } from "./types"

// providerAction is where a form about one provider posts.
export const providerAction = (provider: Provider, what = "") =>
  `/admin/models/providers/${encodeURIComponent(provider.Name)}${what}`

const trigger = <Button variant="outline" size="sm" />

export function AddModel({ provider }: { provider: Provider }) {
  return (
    <FormDialog
      trigger={trigger}
      label="Add model"
      title="Add a model"
      words="A model it answers, and what it costs. A model with no price is not counted against a budget."
      action={providerAction(provider, "/models")}
      submit="Add model"
    >
      <Field id="model-name" label={`Model, as ${provider.Name} names it`}>
        <Input id="model-name" name="model" placeholder="model-name" required />
      </Field>
      <PriceFields
        id="model"
        hints={{
          input: "$ per million",
          output: "$ per million",
          cached: "optional",
        }}
      />
    </FormDialog>
  )
}

export function AddAddress({ provider }: { provider: Provider }) {
  return (
    <FormDialog
      trigger={trigger}
      label="Add address"
      title="Add an address"
      words={`Another address of ${provider.Name}, under the same key.`}
      action={providerAction(provider, "/addresses")}
      submit="Add address"
    >
      <Field id="address-address" label="Address">
        <Input
          id="address-address"
          name="address"
          placeholder="https://api.example.com/anthropic"
          required
        />
      </Field>
      <AddressKind id="address" />
    </FormDialog>
  )
}

export function ReplaceKey({ provider }: { provider: Provider }) {
  return (
    <FormDialog
      trigger={trigger}
      label="Replace key"
      title="Replace the key"
      words="A key is never shown again."
      action={providerAction(provider, "/key")}
      submit="Replace key"
    >
      <Field id="key-new" label="New key">
        <Input
          id="key-new"
          name="key"
          type="password"
          autoComplete="off"
          required
        />
      </Field>
    </FormDialog>
  )
}
