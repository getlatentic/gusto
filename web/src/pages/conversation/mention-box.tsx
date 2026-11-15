import { useState, type KeyboardEvent, type RefObject } from "react"
import { flushSync } from "react-dom"
import { sendOnEnter } from "@/components/chat/composer"
import { UserIcon } from "lucide-react"

import {
  askingAt,
  mention,
  offered,
  pieces,
  type Asking,
} from "@/domain/mentions"
import { cn } from "@/lib/utils"
import type { Mentionable } from "@/pages/conversation/types"

// MentionBox is a room's message box: mentions show in colour as they are typed, and an @ offers the room's agents and people.
// The box's own text is clear, and a copy behind it shows through with its mentions coloured.
export function MentionBox({
  box,
  everyone,
  label,
  placeholder,
  text,
  onWrite,
}: {
  box: RefObject<HTMLTextAreaElement | null>
  everyone: Mentionable[]
  label: string
  placeholder: string
  text: string
  onWrite: (text: string) => void
}) {
  const [asked, setAsked] = useState<Asking>()
  const [active, setActive] = useState(0)
  const choices = asked ? offered(everyone, asked.typed) : []
  const open = choices.length > 0
  const names = everyone.map((each) => each.name)
  const write = (next: string, caret: number) => {
    onWrite(next)
    setAsked(askingAt(next, caret))
    setActive(0)
  }
  const choose = (index: number) => {
    const area = box.current
    const chosen = choices[index]
    if (!area || !asked || !chosen) return
    const put = mention(text, asked, area.selectionStart, chosen.name)
    flushSync(() => {
      onWrite(put.text)
      setAsked(undefined)
    })
    area.setSelectionRange(put.caret, put.caret)
  }
  const keys = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (!open) return
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault()
      setActive(
        (at) =>
          (at + (event.key === "ArrowDown" ? 1 : choices.length - 1)) %
          choices.length
      )
    } else if (event.key === "Enter" || event.key === "Tab") {
      // Choosing with Enter does not also send.
      event.preventDefault()
      event.stopPropagation()
      choose(active)
    } else if (event.key === "Escape") {
      setAsked(undefined)
    }
  }
  return (
    <div className="relative">
      {open && (
        <MentionMenu choices={choices} active={active} onChoose={choose} />
      )}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 overflow-hidden text-sm leading-6 wrap-anywhere whitespace-pre-wrap"
      >
        {pieces(text, names).map((piece, index) => (
          <span
            key={index}
            className={piece.mention ? "text-brand-ink" : undefined}
          >
            {piece.text}
          </span>
        ))}
        {"\n"}
      </div>
      <textarea
        ref={box}
        name="said"
        value={text}
        rows={1}
        aria-label={label}
        placeholder={placeholder}
        role="combobox"
        aria-autocomplete="list"
        aria-controls="mention-menu"
        aria-expanded={open}
        aria-activedescendant={open ? `mention-${active}` : undefined}
        aria-describedby="unsent"
        className="relative block [field-sizing:content] max-h-56 min-h-6 w-full resize-none border-0 bg-transparent p-0 text-sm leading-6 wrap-anywhere whitespace-pre-wrap text-transparent caret-foreground outline-none placeholder:text-muted-foreground max-sm:text-base"
        onChange={(event) =>
          write(event.currentTarget.value, event.currentTarget.selectionStart)
        }
        onClick={(event) =>
          setAsked(
            askingAt(
              event.currentTarget.value,
              event.currentTarget.selectionStart
            )
          )
        }
        onBlur={() => setAsked(undefined)}
        onKeyDownCapture={keys}
        onKeyDown={sendOnEnter}
      />
    </div>
  )
}

function MentionMenu({
  choices,
  active,
  onChoose,
}: {
  choices: Mentionable[]
  active: number
  onChoose: (index: number) => void
}) {
  const agents = choices.filter((each) => each.agent).length
  return (
    <ul
      id="mention-menu"
      role="listbox"
      aria-label="Mention"
      className="absolute bottom-full left-0 z-30 mb-3 grid w-80 max-w-full list-none gap-0.5 rounded-lg border bg-popover p-1 shadow-md"
    >
      {choices.map((each, index) => (
        <li key={each.name} role="presentation" className="contents">
          {(index === 0 || index === agents) && (
            <span
              role="presentation"
              className="px-2.5 pt-1.5 pb-1 text-xs font-medium text-muted-foreground"
            >
              {each.agent ? "Agents" : "People"}
            </span>
          )}
          <span
            id={`mention-${index}`}
            role="option"
            aria-selected={index === active}
            className={cn(
              "flex min-w-0 cursor-pointer items-center gap-2 rounded-md px-2.5 py-1.5 text-sm",
              index === active && "bg-hover"
            )}
            onMouseDown={(event) => {
              event.preventDefault()
              onChoose(index)
            }}
          >
            <span className="grid size-5 flex-none place-items-center overflow-hidden rounded-full border bg-surface text-[0.625rem] font-medium text-muted-foreground">
              {each.icon ? (
                <img
                  src={each.icon}
                  alt=""
                  className="size-3.5 object-contain"
                />
              ) : (
                each.short || <UserIcon className="size-3" />
              )}
            </span>
            <span className="flex-none font-medium">{each.name}</span>
            {each.role && (
              <span className="min-w-0 truncate text-subtle">{each.role}</span>
            )}
            {each.owner && each.owner !== "your agent" && (
              <span className="ml-auto max-w-[45%] flex-none truncate pl-2 text-xs text-subtle">
                {each.owner}
              </span>
            )}
          </span>
        </li>
      ))}
    </ul>
  )
}
