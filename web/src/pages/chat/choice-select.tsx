import { ChevronDownIcon } from "lucide-react"

import { AgentIcon } from "@/components/agent-icon"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
} from "@/components/ui/select"
import type { Option, RoleChoice } from "@/domain/chat-choices"

// A choice is an agent, a model or a preset; a preset menu also has headings, separators and the action that saves one.
type Choice = Option & Pick<RoleChoice, "kind">

// sections splits a menu at its headings, so each heading names the group under it.
function sections(choices: Choice[]): { heading: string; items: Choice[] }[] {
  const out: { heading: string; items: Choice[] }[] = [
    { heading: "", items: [] },
  ]
  for (const choice of choices) {
    if (choice.kind === "heading")
      out.push({ heading: choice.label, items: [] })
    else if (choice.kind !== "separator") out[out.length - 1].items.push(choice)
  }
  return out.filter((section) => section.items.length > 0)
}

// ChoiceSelect is one of the chat's pickers: a quiet button naming what is chosen, and a list with a quieter line under each
// name. It says why it cannot change when it cannot.
export function ChoiceSelect({
  label,
  choices,
  value,
  onChoose,
  fixedBy,
}: {
  label: string
  choices: Choice[]
  value: string
  onChoose: (value: string) => void
  fixedBy?: string
}) {
  const chosen = choices.find(
    (each) => each.value === value && each.kind !== "heading"
  )
  return (
    <Select
      items={choices
        .filter((each) => !each.kind || each.kind === "action")
        .map((each) => ({ value: each.value, label: each.label }))}
      value={value}
      onValueChange={(next) => onChoose(String(next ?? ""))}
      disabled={Boolean(fixedBy)}
    >
      <SelectTrigger
        size="sm"
        aria-label={`${label}: ${chosen?.label ?? "none"}`}
        title={fixedBy || undefined}
        className="max-w-60 rounded-full border-0 bg-transparent text-xs font-medium text-muted-foreground shadow-none hover:bg-hover hover:text-foreground data-[popup-open]:bg-hover [&>svg:last-child]:hidden"
      >
        {chosen?.icon && <AgentIcon src={chosen.icon} className="size-4" />}
        <span className="truncate">{chosen?.label ?? label}</span>
        <ChevronDownIcon aria-hidden="true" className="size-3.5 opacity-60" />
      </SelectTrigger>
      <SelectContent className="max-h-[min(30rem,70dvh)] min-w-72">
        {sections(choices).map((section, index) => (
          <SelectGroup key={section.heading || index}>
            {index > 0 && <SelectSeparator />}
            {section.heading && <SelectLabel>{section.heading}</SelectLabel>}
            {section.items.map((choice) => (
              <SelectItem
                key={choice.value}
                value={choice.value}
                className="items-start py-2"
              >
                {choice.icon && <AgentIcon src={choice.icon} />}
                <span className="grid gap-0.5">
                  <span>{choice.label}</span>
                  {choice.detail && (
                    <span className="text-xs text-muted-foreground">
                      {choice.detail}
                    </span>
                  )}
                </span>
              </SelectItem>
            ))}
          </SelectGroup>
        ))}
      </SelectContent>
    </Select>
  )
}
