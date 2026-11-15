import { useState } from "react"
import { CheckIcon, CopyIcon, ReplyIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { useConversation } from "@/pages/conversation/conversation-context"

// MessageTools are what can be done with a message, shown on hover or focus: copy its words, and answer whoever sent it.
export function MessageTools({
  copy,
  answer,
}: {
  copy: () => string
  answer?: string
}) {
  const { answer: reply } = useConversation()
  const [copied, setCopied] = useState(false)
  return (
    <div
      role="toolbar"
      aria-label="Message actions"
      className="pointer-events-none absolute -top-4 right-0 z-10 flex gap-0.5 rounded-lg border bg-surface p-0.5 opacity-0 transition-opacity group-focus-within/msg:pointer-events-auto group-focus-within/msg:opacity-100 group-hover/msg:pointer-events-auto group-hover/msg:opacity-100 [@media(hover:none)]:hidden"
    >
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={copied ? "Copied" : "Copy"}
        title={copied ? "Copied" : "Copy"}
        onClick={() => {
          void navigator.clipboard?.writeText(copy())
          setCopied(true)
          window.setTimeout(() => setCopied(false), 1_500)
        }}
      >
        {copied ? <CheckIcon className="text-ok" /> : <CopyIcon />}
      </Button>
      {answer && (
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label={`Reply to ${answer}`}
          title="Reply"
          onClick={() => reply(answer)}
        >
          <ReplyIcon />
        </Button>
      )}
    </div>
  )
}
