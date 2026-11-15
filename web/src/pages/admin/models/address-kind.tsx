import { ChevronRightIcon } from "lucide-react"

import { Checkbox } from "@/components/ui/checkbox"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { Label } from "@/components/ui/label"
import { ChoiceSelect } from "./choice-select"
import { Field } from "./form-dialog"

const kinds = [
  { value: "openai", label: "OpenAI-compatible" },
  { value: "anthropic", label: "Anthropic-compatible" },
  { value: "custom", label: "Custom" },
]

const auths = [
  { value: "bearer", label: "a bearer token" },
  { value: "anthropic", label: "Anthropic's header" },
]

const wires = [
  { value: "chat", label: "Chat Completions" },
  { value: "responses", label: "Responses" },
  { value: "messages", label: "Messages" },
]

// AddressKind is the kind of API an address answers, and for a custom one, how it takes its key and the shapes it answers.
// The custom part stays in the form while it is folded, so the form sends the same fields either way.
export function AddressKind({ id }: { id: string }) {
  return (
    <>
      <Field id={`${id}-kind`} label="Kind of API">
        <ChoiceSelect id={`${id}-kind`} name="kind" choices={kinds} />
      </Field>
      <Collapsible className="w-full">
        <CollapsibleTrigger className="group/kind flex items-center gap-1.5 text-left text-sm text-muted-foreground hover:text-foreground">
          <ChevronRightIcon
            className="size-4 flex-none transition-transform group-data-[panel-open]/kind:rotate-90"
            aria-hidden="true"
          />
          Custom: the key&apos;s header and the shapes it answers
        </CollapsibleTrigger>
        <CollapsibleContent keepMounted className="mt-2 flex flex-wrap gap-4">
          <Field id={`${id}-auth`} label="Sends its key as">
            <ChoiceSelect
              id={`${id}-auth`}
              name="auth"
              choices={auths}
              className="w-fit"
            />
          </Field>
          {wires.map((wire) => (
            <Label key={wire.value} className="font-normal">
              <Checkbox name="wires" value={wire.value} />
              {wire.label}
            </Label>
          ))}
        </CollapsibleContent>
      </Collapsible>
    </>
  )
}
