import { Fragment, useRef, useState } from "react"
import { flushSync } from "react-dom"

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { cn } from "@/lib/utils"

export interface FormChoice {
  value: string
  label: string
}

// A group of choices under a heading; a group with no heading is drawn first, as the options before any optgroup are.
export interface ChoiceGroup {
  heading?: string
  choices: FormChoice[]
}

// FormSelect is a select of a form, sent under its name as a native field is. One that sends its form sends it as soon as a
// choice is made, as the template's autosubmitting selects did. Give it a key of the value the server holds, so a page read
// again starts it there.
export function FormSelect({
  id,
  name,
  label,
  groups,
  value,
  sendOnChoose = false,
  className,
  testId,
}: {
  id?: string
  name: string
  label?: string
  groups: ChoiceGroup[]
  value: string
  sendOnChoose?: boolean
  className?: string
  testId?: string
}) {
  const trigger = useRef<HTMLButtonElement>(null)
  const [chosen, setChosen] = useState(value)
  const choose = (next: unknown) => {
    // The form's field holds the choice once it is drawn with it, and only then is the form sent.
    flushSync(() => setChosen(String(next ?? "")))
    if (sendOnChoose) trigger.current?.closest("form")?.requestSubmit()
  }
  return (
    <Select
      name={name}
      items={groups.flatMap((group) => group.choices)}
      value={chosen}
      onValueChange={choose}
    >
      <SelectTrigger
        ref={trigger}
        id={id}
        aria-label={label}
        data-testid={testId}
        className={cn("w-full", className)}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {groups.map((group, index) => (
          <Fragment key={group.heading ?? index}>
            {index > 0 && <SelectSeparator />}
            <SelectGroup>
              {group.heading && <SelectLabel>{group.heading}</SelectLabel>}
              {group.choices.map((choice) => (
                <SelectItem key={choice.value} value={choice.value}>
                  {choice.label}
                </SelectItem>
              ))}
            </SelectGroup>
          </Fragment>
        ))}
      </SelectContent>
    </Select>
  )
}
