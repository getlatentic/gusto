import { useEffect, useRef, useState } from "react"
import { SearchIcon } from "lucide-react"

import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"

// SearchBox searches as the person types, once they pause, and at once on Enter.
export function SearchBox({
  label,
  initial,
  onSearch,
}: {
  label: string
  initial: string
  onSearch: (q: string) => void
}) {
  const [text, setText] = useState(initial)
  const sent = useRef(initial)
  useEffect(() => {
    if (text === sent.current) return
    const wait = setTimeout(() => {
      sent.current = text
      onSearch(text)
    }, 400)
    return () => clearTimeout(wait)
  }, [text, onSearch])
  return (
    <InputGroup className="h-8 w-60 max-sm:w-full">
      <InputGroupAddon>
        <SearchIcon />
      </InputGroupAddon>
      <InputGroupInput
        type="search"
        aria-label={label}
        placeholder="Search"
        value={text}
        onChange={(event) => setText(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter" && text !== sent.current) {
            sent.current = text
            onSearch(text)
          }
        }}
      />
    </InputGroup>
  )
}
