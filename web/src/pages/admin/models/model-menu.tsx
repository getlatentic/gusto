import { useRef, useState, type RefObject } from "react"
import { EllipsisIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { FormDialog } from "@/components/form-dialog"
import { PriceFields } from "./price-fields"
import { providerAction } from "./provider-dialogs"
import type { Provider, ProviderModel } from "./types"

// ModelMenu is what can be done to one model of the organization's provider: price it, turn it on or off, or retire it. Each
// is a form of the row's own, which the menu sends.
export function ModelMenu({
  provider,
  model,
}: {
  provider: Provider
  model: ProviderModel
}) {
  const [pricing, setPricing] = useState(false)
  const turn = useRef<HTMLFormElement>(null)
  const retire = useRef<HTMLFormElement>(null)
  const turnTo = model.Pending ? "enable" : "disable"
  return (
    <>
      <ModelForm
        form={turn}
        action={providerAction(provider, `/models/${turnTo}`)}
        model={model}
      />
      <ModelForm
        form={retire}
        action={providerAction(provider, "/models/retire")}
        model={model}
      />
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`More for ${model.Full}`}
            />
          }
        >
          <EllipsisIcon />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-40">
          <DropdownMenuItem onClick={() => setPricing(true)}>
            Set price
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => turn.current?.requestSubmit()}>
            {model.Pending ? "Turn on" : "Turn off"}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            variant="destructive"
            onClick={() => retire.current?.requestSubmit()}
          >
            Retire
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <FormDialog
        open={pricing}
        onOpenChange={setPricing}
        title={`Price of ${model.Full}`}
        words="Dollars for a million tokens. Leave them all empty for no price, so its cost is not counted against a cap."
        action={providerAction(provider, "/price")}
        submit="Set price"
      >
        <input type="hidden" name="model" value={model.Name} />
        <PriceFields
          id="price"
          price={model.Price}
          hints={{ input: "no price", output: "no price", cached: "as input" }}
        />
      </FormDialog>
    </>
  )
}

function ModelForm({
  form,
  action,
  model,
}: {
  form: RefObject<HTMLFormElement | null>
  action: string
  model: ProviderModel
}) {
  return (
    <form ref={form} method="post" action={action} hidden>
      <input type="hidden" name="model" value={model.Name} />
    </form>
  )
}
