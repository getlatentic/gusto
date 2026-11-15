import { useState, type FormEvent } from "react"
import { useNavigate } from "react-router"

import { isAppPage } from "@/components/app-link"
import { useRefreshAll } from "@/lib/api"
import { sendForm } from "@/lib/forms"

// useWorkForm sends one of the work pages' forms without leaving the page, then goes where the server sent the person, where
// any refusal is said: within the app when the app draws that page, and by loading it when the server does. Every page's data
// is read again, since what changed here shows in the side list and on the other pages too.
export function useWorkForm() {
  const navigate = useNavigate()
  const refreshAll = useRefreshAll()
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
    if (!landed) return
    if (isAppPage(landed)) navigate(landed)
    else window.location.assign(landed)
  }
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    void send(event.currentTarget)
  }
  return { submit, send, failed, sending }
}
