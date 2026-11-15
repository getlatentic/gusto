import { useState, type FormEvent } from "react"
import { useLocation, useNavigate } from "react-router"

import { useRefreshAll } from "@/lib/api"
import { sendForm } from "@/lib/forms"

// useAccountForm sends one of the account pages' forms without leaving the page, then moves to where the server sent the
// person, which says what became of it, and reads the pages' data again. A form the server turned away outright leaves its
// reason in failed.
export function useAccountForm() {
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

export type AccountForm = ReturnType<typeof useAccountForm>
