import { useState } from "react"

import { Button } from "@/components/ui/button"
import { tellCloud } from "@/lib/cloud"
import { api } from "@/store/api"

interface Account {
  confirmed: boolean
  email: string
  jobs: number
}

// sentWords is what to tell the person after asking for the link again.
function sentWords(status: number): string {
  if (status >= 200 && status < 300) return "Sent. Check your inbox."
  if (status === 429)
    return "That address has been mailed too often. Try again later."
  return "The link could not be sent. Try again."
}

// EmailReminder asks a person who has not confirmed their address to, and mails the link again. Only gusto cloud knows
// whether they have.
export function EmailReminder({ from, send }: { from: string; send: string }) {
  const account = api.endpoints.cloud.useQuery(from).data as Account | undefined
  // again is gusto cloud's answer to sending the link again: its status, or 0 when it could not be asked.
  const [again, setAgain] = useState<{ sending: boolean; status?: number }>({
    sending: false,
  })
  const sendAgain = () => {
    setAgain({ sending: true })
    tellCloud(send).then(
      (status) => setAgain({ sending: false, status }),
      () => setAgain({ sending: false, status: 0 })
    )
  }
  if (!account || account.confirmed) return null
  // Once the link is on its way there is nothing more to send.
  const sent =
    again.status !== undefined && again.status >= 200 && again.status < 300
  const said = again.status === undefined ? "" : sentWords(again.status)
  return (
    <aside
      aria-label="Confirm your email address"
      className="grid gap-2 rounded-md border bg-surface p-3 text-sm"
    >
      <p className="m-0 font-medium">Confirm your email address</p>
      <p className="m-0 text-muted-foreground">
        It lifts the trial to {account.jobs} tasks a month and lets you invite
        colleagues. We mailed a link to{" "}
        <span className="wrap-anywhere text-foreground">{account.email}</span>.
      </p>
      {said && (
        <p className="m-0 text-muted-foreground" role="status">
          {said}
        </p>
      )}
      <Button
        variant="outline"
        size="sm"
        disabled={again.sending || sent}
        onClick={sendAgain}
      >
        Send it again
      </Button>
    </aside>
  )
}
