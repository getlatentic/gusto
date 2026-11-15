import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { AddressKind } from "./address-kind"
import { ChoiceSelect } from "./choice-select"
import { Field, FormDialog } from "@/components/form-dialog"
import type { ModelsView } from "./types"

// AddProvider adds a provider: where its models answer, and where its key comes from.
export function AddProvider({ view }: { view: ModelsView }) {
  const sources = (view.Kinds ?? ["stored"]).map((kind) => ({
    value: kind,
    label: kind === "stored" ? "typed here, kept and never shown" : kind,
  }))
  return (
    <FormDialog
      trigger={<Button variant="outline" />}
      label="Add a provider"
      title="Add a provider"
      words="Where models answer and how to be let in."
      action="/admin/models/providers"
      submit="Add provider"
    >
      {!view.Private && (
        <p className="m-0 text-sm text-muted-foreground">
          It must be at a public https address.
        </p>
      )}
      <Field id="provider-name" label="Name">
        <Input
          id="provider-name"
          name="name"
          placeholder="acme"
          pattern="[a-z][a-z0-9\-]*"
          maxLength={31}
          required
        />
      </Field>
      <Field id="provider-address" label="Address">
        <Input
          id="provider-address"
          name="address"
          placeholder="https://api.example.com"
          required
        />
      </Field>
      <AddressKind id="provider" />
      <Field id="provider-source" label="Key comes from">
        <ChoiceSelect id="provider-source" name="source" choices={sources} />
      </Field>
      <Field id="provider-key" label="Key">
        <Input
          id="provider-key"
          name="key"
          type="password"
          autoComplete="off"
        />
      </Field>
      <Field
        id="provider-reference"
        label="Reference, when the key is not typed here"
      >
        <Input
          id="provider-reference"
          name="reference"
          placeholder="/path, parameter name or secret id"
        />
      </Field>
    </FormDialog>
  )
}
