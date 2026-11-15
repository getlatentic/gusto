import { useState, type FormEvent } from "react"
import { useLocation, useNavigate } from "react-router"

import { useRefreshAll } from "@/lib/api"
import { sendForm } from "@/lib/forms"

// useMembersForm sends one of the members pages' forms without leaving the page, then shows what the server said: it moves to
// where the server sent the person (a refusal is said there), and reads every page's data again, since an agent changed here
// shows in rooms and the side list too. A form the server turned away outright leaves its reason in failed.
export function useMembersForm() {
  const navigate = useNavigate()
  const refreshAll = useRefreshAll()
  const here = useLocation()
  const [failed, setFailed] = useState("")
  const [sending, setSending] = useState(false)
  const send = async (form: HTMLFormElement) => {
    let landed: string
    setFailed("")
    setSending(true)
    try {
      landed = await sendForm(form)
    } catch (err) {
      setFailed(err instanceof Error ? err.message : String(err))
      return
    } finally {
      setSending(false)
    }
    refreshAll()
    if (landed)
      navigate(landed, { replace: landed === here.pathname + here.search })
  }
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    void send(event.currentTarget)
  }
  return { submit, send, failed, sending }
}
