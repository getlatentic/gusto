import { useState, type FormEvent } from "react"
import { useNavigate } from "react-router"

import { sendForm } from "@/lib/forms"
import type { ConversationData } from "@/pages/conversation/types"
import { useRefreshAll } from "@/lib/api"

// useRoomForm sends one of the room's forms without leaving the page, then shows what the server said: it moves to where the
// server sent the person (a refusal is said there, with its dialog open), and reads the room, its turns and the side list again.
// A form the server turned away outright (not the person's to send) leaves its reason in failed.
export function useRoomForm(page: ConversationData) {
  const navigate = useNavigate()
  const refreshAll = useRefreshAll()
  const [failed, setFailed] = useState("")
  const send = async (form: HTMLFormElement) => {
    let landed: string
    setFailed("")
    try {
      landed = await sendForm(form)
    } catch (err) {
      setFailed(err instanceof Error ? err.message : String(err))
      return
    }
    // A room change can show anywhere: in the room, its turns and the side list.
    refreshAll()
    if (landed)
      navigate(landed, { replace: landed.split("?")[0] === page.Path })
  }
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    await send(event.currentTarget)
  }
  // post sends a form made here, for an action that is a menu item rather than a form on the page.
  const post = (action: string, fields: Record<string, string> = {}) => {
    const form = document.createElement("form")
    form.method = "post"
    form.action = action
    for (const [name, value] of Object.entries(fields)) {
      const field = document.createElement("input")
      field.name = name
      field.value = value
      form.append(field)
    }
    return send(form)
  }
  return { submit, post, send, failed }
}
