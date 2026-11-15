import { Button } from "@/components/ui/button"

// starters are things a person might ask first; choosing one writes it in the box, and sends nothing.
const starters = [
  {
    label: "Draft a policy for using AI at work",
    said: "Draft a one-page policy for how we use AI at work",
  },
  { label: "Summarize this folder", said: "Summarize this folder" },
  {
    label: "Research competitors and write a brief",
    said: "Research three competitors and write me a short brief",
  },
]

export function Starters({ onChoose }: { onChoose: (said: string) => void }) {
  return (
    <ul
      aria-label="Ways to start"
      className="m-0 mx-auto mt-6 flex max-w-180 list-none flex-wrap justify-center gap-2 p-0 max-sm:order-first max-sm:mx-0 max-sm:mt-0 max-sm:mb-3 max-sm:w-full max-sm:[scrollbar-width:none] max-sm:flex-nowrap max-sm:justify-start max-sm:overflow-x-auto"
    >
      {starters.map((starter) => (
        <li key={starter.label} className="flex-none">
          <Button
            type="button"
            variant="outline"
            className="rounded-full font-normal"
            onClick={() => onChoose(starter.said)}
          >
            {starter.label}
          </Button>
        </li>
      ))}
    </ul>
  )
}
