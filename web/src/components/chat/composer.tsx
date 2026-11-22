import type { KeyboardEvent, ReactNode, Ref } from "react"
import { ArrowUpIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

// grow fits the box to what is written in it, up to its most.
function grow(box: HTMLTextAreaElement) {
  box.style.height = "auto"
  box.style.height = `${box.scrollHeight}px`
}

// sendOnEnter sends with Enter and makes a new line with Shift+Enter; an empty box sends nothing.
export function sendOnEnter(event: KeyboardEvent<HTMLTextAreaElement>) {
  if (
    event.defaultPrevented ||
    event.key !== "Enter" ||
    event.shiftKey ||
    event.nativeEvent.isComposing
  )
    return
  event.preventDefault()
  if (event.currentTarget.value.trim())
    event.currentTarget.form?.requestSubmit()
}

// Composer is the box a person writes in: the message, then a row of what it goes with, and Send. The message and the row start
// at one edge, and the box has the same room on every side.
export function Composer({
  label,
  placeholder,
  defaultValue,
  text,
  field,
  describedBy,
  autoFocus,
  canSend,
  onWrite,
  boxRef,
  tools,
  actions,
  send,
  below,
  className,
}: {
  label: string
  placeholder: string
  defaultValue?: string
  // text makes the box hold what its owner keeps, and field replaces the box altogether, as a room's box with mentions does.
  text?: string
  field?: ReactNode
  describedBy?: string
  autoFocus?: boolean
  canSend: boolean
  onWrite?: (text: string) => void
  boxRef?: Ref<HTMLTextAreaElement>
  tools?: ReactNode
  actions?: ReactNode
  // send, when it is given, takes the send button's place, as Stop does while a reply is being written.
  send?: ReactNode
  below?: ReactNode
  className?: string
}) {
  return (
    <fieldset
      className={cn(
        "m-0 grid min-w-0 gap-2 rounded-lg border border-input bg-surface p-3 shadow-xs transition-[border-color,box-shadow] has-[textarea:focus-visible]:border-foreground/20",
        className
      )}
    >
      {field ?? (
        <textarea
          ref={boxRef}
          data-testid="message-box"
          name="said"
          rows={1}
          aria-label={label}
          aria-describedby={describedBy}
          placeholder={placeholder}
          defaultValue={text === undefined ? defaultValue : undefined}
          value={text}
          autoFocus={autoFocus}
          required
          className="block [field-sizing:content] max-h-56 min-h-6 w-full resize-none border-0 bg-transparent p-0 text-sm leading-6 outline-none placeholder:text-muted-foreground max-sm:text-base"
          onChange={(event) => {
            grow(event.currentTarget)
            onWrite?.(event.currentTarget.value)
          }}
          onKeyDown={sendOnEnter}
        />
      )}
      <div className="flex min-w-0 items-center gap-1">
        {tools}
        <span className="flex-1" />
        {actions}
        {send ?? (
          <Button
            type="submit"
            data-testid="send"
            size="icon-sm"
            className="rounded-full"
            disabled={!canSend}
            aria-label="Send"
            title="Send"
          >
            <ArrowUpIcon />
          </Button>
        )}
      </div>
      {below}
    </fieldset>
  )
}
