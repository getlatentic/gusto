import { createContext, useContext, type FormEvent } from "react"

import type { ConversationData } from "@/pages/conversation/types"

// ConversationContext is what any part of one conversation may ask of the page around it: the page's data, to answer someone
// by name in the box, to open a file beside the conversation, and to stop a reply. It is provided by the conversation page
// and by nothing else.
export interface Conversation {
  page: ConversationData
  admin: boolean
  // answer puts "@Name " before what is written in the box, and puts the caret in it.
  answer: (name: string) => void
  // openFile opens a file in the panel beside the conversation, and says whether it did (it does not on a narrow window).
  openFile: (href: string, name: string) => boolean
  stop: (ref: string, form: HTMLFormElement) => void
  // send sends a form of the page as it stands, without leaving the page.
  send: (form: HTMLFormElement) => Promise<void>
  // submit sends one of the room's forms without leaving the page.
  submit: (event: FormEvent<HTMLFormElement>) => Promise<void>
  post: (action: string, fields?: Record<string, string>) => Promise<void>
}

export const ConversationContext = createContext<Conversation | null>(null)

export function useConversation(): Conversation {
  const held = useContext(ConversationContext)
  if (!held)
    throw new Error("useConversation is used outside a conversation page")
  return held
}
