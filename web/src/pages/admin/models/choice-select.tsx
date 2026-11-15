import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

export interface Choice {
  value: string
  label: string
}

// ChoiceSelect is a select of a form, sent under its name as a native field is.
export function ChoiceSelect({
  id,
  name,
  choices,
  defaultValue = choices[0]?.value,
  className = "w-full",
}: {
  id: string
  name: string
  choices: Choice[]
  defaultValue?: string
  className?: string
}) {
  return (
    <Select name={name} items={choices} defaultValue={defaultValue}>
      <SelectTrigger id={id} className={className}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {choices.map((choice) => (
          <SelectItem key={choice.value} value={choice.value}>
            {choice.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
